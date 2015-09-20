package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joyrexus/todos"
)

const verbose = false // if `true` you'll see log output

func main() {
	// Start our todos server and cleanup afterward.
	dbfile := "todos.db" // path to file to use for persisting todos
	srv := todos.NewServer(dbfile)
	defer srv.Close()
	defer os.Remove(dbfile)

	// Setup daily todos for client to post.
	posts := []*Todo{
		&Todo{Day: "mon", Task: "milk cows"},
		&Todo{Day: "mon", Task: "feed cows"},
		&Todo{Day: "mon", Task: "wash cows"},
		&Todo{Day: "tue", Task: "wash laundry"},
		&Todo{Day: "tue", Task: "fold laundry"},
		&Todo{Day: "tue", Task: "iron laundry"},
		&Todo{Day: "wed", Task: "flip burgers"},
		&Todo{Day: "thu", Task: "join army"},
		&Todo{Day: "fri", Task: "kill time"},
		&Todo{Day: "sat", Task: "have beer"},
		&Todo{Day: "sat", Task: "make merry"},
		&Todo{Day: "sun", Task: "take aspirin"},
		&Todo{Day: "sun", Task: "pray quietly"},
	}

	// Create our helper http client.
	client := new(Client)

	// Use our client to post each daily todo.
	for _, todo := range posts {
		url := srv.URL + "/day/" + todo.Day
		if err := client.post(url, todo); err != nil {
			fmt.Printf("client post error: %v", err)
		}
	}

	// Now, let's try retrieving the persisted todos.

	// Get a list of tasks for each day.
	week := []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}
	fmt.Println("daily tasks ...")
	for _, day := range week {
		url := srv.URL + "/day/" + day
		tasks, err := client.get(url)
		if err != nil {
			fmt.Printf("client get error: %v", err)
		}
		fmt.Printf("  %s: %s\n", day, tasks)
	}
	// Output:
	// daily tasks ...
	//   mon: milk cows, feed cows, wash cows
	//   tue: wash laundry, fold laundry, iron laundry
	//   wed: flip burgers
	//   thu: join army
	//   fri: kill time
	//   sat: have beer, make merry
	//   sun: take aspirin, pray quietly

	// Get a list of combined tasks for weekdays.
	tasks, err := client.get(srv.URL + "/weekdays")
	if err != nil {
		fmt.Printf("client get error: %v", err)
	}
	fmt.Printf("\nweekday tasks: %s\n", tasks)
	// Output:
	// weekday tasks: milk cows, feed cows, wash cows, wash laundry,
	// fold laundry, iron laundry, flip burgers, join army, kill time

	// Get a list of combined tasks for the weekend.
	tasks, err = client.get(srv.URL + "/weekend")
	if err != nil {
		fmt.Printf("client get error: %v", err)
	}
	fmt.Printf("\nweekend tasks: %s\n", tasks)
	// Output:
	// weekend tasks: have beer, make merry, take aspirin, pray quietly
}

/* -- CLIENT -- */

// Our http client for sending requests.
type Client struct{}

// post sends a post request with a json payload.
func (c *Client) post(url string, todo *Todo) error {
	todo.Created = time.Now()
	bodyType := "application/json"
	body, err := todo.Encode()
	if err != nil {
		return err
	}
	resp, err := http.Post(url, bodyType, body)
	if err != nil {
		return err
	}
	if verbose {
		log.Printf("client: %s\n", resp.Status)
	}
	return nil
}

// get sends get requests and expects responses to be a json-encoded
// task list.
func (c *Client) get(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	taskList := new(TaskList)
	if err = json.NewDecoder(resp.Body).Decode(taskList); err != nil {
		return "", err
	}
	return strings.Join(taskList.Tasks, ", "), nil
}

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

// A TaskList is a list of tasks for a particular day ("monday") or 
// set of days ("weekdays", "weekend").
type TaskList struct {
	When  string
	Tasks []string
}
