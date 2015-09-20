package todos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/joyrexus/buckets"
	"github.com/julienschmidt/httprouter"
)

const verbose = false // if `true` you'll see log output

func NewServer(bxPath string) *Server {
	// Open a buckets database.
	bx, err := buckets.Open(bxPath)
	if err != nil {
		log.Fatalf("couldn't open buckets db %q: %v", bxPath, err)
	}

	// Create/open bucket for storing todos.
	bucket, err := bx.New([]byte("todos"))
	if err != nil {
		log.Fatalf("couldn't create/open todos bucket: %v", err)
	}

	// Initialize our controller for handling specific routes.
	control := NewController(bucket)

	// Create and setup our router.
	mux := httprouter.New()
	mux.POST("/day/:day", control.post)
	mux.GET("/day/:day", control.getDayTasks)
	mux.GET("/weekend", control.getWeekendTasks)
	mux.GET("/weekdays", control.getWeekdayTasks)

	// Start our web server.
	srv := httptest.NewServer(mux)
	return &Server{srv.URL, bx, srv}
}

type Server struct {
	URL string
	buckets    *buckets.DB
	httpserver *httptest.Server
}

func (s *Server) Close() {
	s.buckets.Close()
	s.httpserver.Close()
}

/* -- MODELS --*/

// A Todo models a daily task.
type Todo struct {
	Task    string    // task to be done
	Day     string    // day to do task
	Created time.Time // when created
}

// Encode marshals a Todo into a json-encoded r/w buffer.
func (todo *Todo) Encode() (*bytes.Buffer, error) {
	b, err := json.Marshal(todo)
	if err != nil {
		return nil, err
	}
	return bytes.NewBuffer(b), nil
}

// A TaskList is a list of tasks for a particular day.
type TaskList struct {
	When  string
	Tasks []string
}

/* -- CONTROLLER -- */

// NewController initializes a new instance of our controller.
// It provides handler methods for our router.
func NewController(bk *buckets.Bucket) *Controller {
	// map of days to integers
	daynum := map[string]int{
		"mon": 1, // monday is the first day of the week
		"tue": 2,
		"wed": 3,
		"thu": 4,
		"fri": 5,
		"sat": 6,
		"sun": 7,
	}
	// map of prefix scanners for iterating over keys with prefixes
	prefixscan := map[string]*buckets.PrefixScanner{
		"mon": bk.NewPrefixScanner([]byte("1")),
		"tue": bk.NewPrefixScanner([]byte("2")),
		"wed": bk.NewPrefixScanner([]byte("3")),
		"thu": bk.NewPrefixScanner([]byte("4")),
		"fri": bk.NewPrefixScanner([]byte("5")),
		"sat": bk.NewPrefixScanner([]byte("6")),
		"sun": bk.NewPrefixScanner([]byte("7")),
	}
	// map of range scanners for iterating over keys within ranges
	rangescan := map[string]*buckets.RangeScanner{
		// weekdays are mon to fri: 1 <= key < 6.
		"weekday": bk.NewRangeScanner([]byte("1"), []byte("6")),
		// weekends are sat to sun: 6 <= key < 8.
		"weekend": bk.NewRangeScanner([]byte("6"), []byte("8")),
	}
	return &Controller{bk, daynum, prefixscan, rangescan}
}

// This Controller handles requests for todo items.  The items are stored
// in a todos bucket.  The request URLs are used as bucket keys and the
// raw json payload as values.
//
// Note that since we're using `httprouter` (abbreviated as `mux` when
// imported) as our router, each method is a `httprouter.Handle` rather
// than a `http.HandlerFunc`.
type Controller struct {
	todos      *buckets.Bucket
	daynum     map[string]int
	prefixscan map[string]*buckets.PrefixScanner
	rangescan  map[string]*buckets.RangeScanner
}

// getWeekendTasks handles get requests for `/weekend`, returning the
// combined task list for saturday and sunday.
//
// Note how we utilize the `weekend` range scanner, which makes it easy
// to iterate over keys in our todos bucket within a certain range,
// viz. those keys from saturday (day number 6) to sunday (7).
func (c *Controller) getWeekendTasks(w http.ResponseWriter, r *http.Request,
	_ httprouter.Params) {

	// Get todo items within the weekend range.
	items, err := c.rangescan["weekend"].Items()
	if err != nil {
		http.Error(w, err.Error(), 500)
	}

	// Generate a list of tasks based on todo items retrieved.
	taskList := &TaskList{"weekend", []string{}}

	for _, item := range items {
		todo, err := decode(item.Value)
		if err != nil {
			http.Error(w, err.Error(), 500)
		}
		taskList.Tasks = append(taskList.Tasks, todo.Task)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(taskList)
}

// getWeekdayTasks handles get requests for `/weekdays`, returning the
// combined task list for monday through friday.
//
// Note how we utilize the `weekday` range scanner, which makes it easy
// to iterate over keys in our todos bucket within a certain range,
// viz. those keys from monday (day number 1) to friday (5).
func (c *Controller) getWeekdayTasks(w http.ResponseWriter, r *http.Request,
	_ httprouter.Params) {

	// Get todo items within the weekday range.
	items, err := c.rangescan["weekday"].Items()
	if err != nil {
		http.Error(w, err.Error(), 500)
	}

	// Generate a list of tasks based on todo items retrieved.
	taskList := &TaskList{"weekdays", []string{}}

	for _, item := range items {
		todo, err := decode(item.Value)
		if err != nil {
			http.Error(w, err.Error(), 500)
		}
		taskList.Tasks = append(taskList.Tasks, todo.Task)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(taskList)
}

// getDayTasks handles get requests for `/:day`, returning a particular
// day's task list.
//
// Note how we utilize the prefix scanner for the day requested (as indicated
// in the route's `day` parameter). This makes it easy to iterate over keys
// in our todos bucket with a certain prefix, viz. those with the prefix
// representing the requested day.
func (c *Controller) getDayTasks(w http.ResponseWriter, r *http.Request,
	p httprouter.Params) {

	// Get todo items for the day requested.
	day := p.ByName("day")
	items, err := c.prefixscan[day].Items()
	if err != nil {
		http.Error(w, err.Error(), 500)
	}

	// Generate a list of tasks based on todo items retrieved.
	taskList := &TaskList{day, []string{}}

	for _, item := range items {
		todo, err := decode(item.Value)
		if err != nil {
			http.Error(w, err.Error(), 500)
		}
		taskList.Tasks = append(taskList.Tasks, todo.Task)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(taskList)
}

// post handles post requests to create a daily todo item.
//
//
func (c *Controller) post(w http.ResponseWriter, r *http.Request,
	p httprouter.Params) {

	// Read request body's json payload into buffer.
	b, err := ioutil.ReadAll(r.Body)
	todo, err := decode(b)
	if err != nil {
		http.Error(w, err.Error(), 500)
	}

	// Use the day number + creation time as key.
	day := p.ByName("day")
	num := c.daynum[day] // number of day of week
	created := todo.Created.Format(time.RFC3339Nano)
	key := fmt.Sprintf("%d/%s", num, created)

	// Put key/buffer into todos bucket.
	if err := c.todos.Put([]byte(key), b); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if verbose {
		log.Printf("server: %s: %v", key, todo.Task)
	}

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "put todo for %s: %s\n", key, todo)
}

/* -- UTILITY FUNCTIONS, &c. -- */

// decode unmarshals a json-encoded byteslice into a Todo.
func decode(b []byte) (*Todo, error) {
	todo := new(Todo)
	if err := json.Unmarshal(b, todo); err != nil {
		return nil, err
	}
	return todo, nil
}
