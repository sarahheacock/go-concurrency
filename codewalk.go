package main
// polls a list of URLS, checking their HTTP res codes
// periodically printing their state

import (
  "log"
  "net/http"
  "time"
)

// diff from threads ->
// Don't communicate by sharing memory
// share memory by communicating

// channels allow you to pass ref to data structures
// between goroutines
const (
  numPollers = 2 // # of goroutines to launch
  pollInterval = 60 * time.Second // how often to poll each URL
  statusInterval = 10 * time.Second // how often to log status
  errTimeout = 10 * time.Second // back-off timeout on error
)

var urls = []string{
  "http://www.google.com",
  "http://golang.org",
  "http://blog.golang.org",
}

// STATE TYPE
// State Type represents state of a URL
// the Pollers send State values to StateMonitor
// which maintains map of current state of each URL
type State struct {
  url string
  status string
}

// STATEMONITOR
// maintains a map that sotres the state of the URLs being
// polled, and prints the current state every updateInterval nanoseconds.
// It returns a chan State to which resource state should be sent
func StateMonitor(updateInterval time.Duration) chan<- State {
  // where goroutine Poller sends State values
  updates := make(chan State)

  // map of urls to most recent status
  urlStatus := make(map[string]string)

  // object that repeatedly sends a value on a channel at specified time
  ticker := time.NewTicker(updateInterval)

  // StateMonitor will loop forever selecting on two channels (ticker.C and update)
  // select statement blocks untl one of its communications is read to proceed
  // When StateMonitor receives a tick from ticker.C, it logs state
  go func() {
    for {
      select {
      case <-ticker.C:
        logState(urlStatus)
      case s := <-updates:
        urlStatus[s.url] = s.status
      }
    }
  }()
  return updates
}

// logState prints a state map
func logState(s map[string]string) {
  log.Println("Current state:")
  for k, v := range s {
    log.Printf(" %s %s", k, v)
  }
}

// RESOURCE TYPE
// A Resource represents the state of a URL to be polled
// includes the url and number of errors since last poll
// When program starts, allocates on Resource for each URL
type Resource struct {
  url string
  errCount int
}

// RESOURCE'S METHODS
// performs HTTP HEAD request for Resource's URL
// and returns HTTP response status
func (r *Resource) Poll() string {
  resp, err := http.Head(r.url)
  if err != nil {
    log.Println("Error", r.url, err)
    r.errCount++
    return err.Error()
  }
  r.errCount = 0
  return resp.Status
}

// Sleep sleeps for an interval
// before sending the Resource to done
func (r *Resource) Sleep(done chan<- *Resource) {
  time.Sleep(pollInterval + errTimeout*time.Duration(r.errCount))
  done <- r
}

// POLLER FUNCTION
// Each Poller receiveds Resource pointers from input channel
// Passes ownership of underlying data from sender to receiver (don't have to worry about locking)
// Sends State value to status channel to inform StateMonitor result of Poll
// Finally sends Resource to out channel and "returns ownership" to main goroutine
func Poller(in <-chan *Resource, out chan<- *Resource, status chan<- State){
  for r := range in {
    s := r.Poll()
    status <- State{r.url, s}
    out <- r
  }
}

// MAIN FUNCTION
// starts Poller and StateMonitor goroutines
// passes completed resources back to pending channel
// after appropriate delays
func main() {
  // ceate input and output channels
  pending, complete := make(chan *Resource), make(chan *Resource)

  // launch StateMonitor
  // goroutine that stores the state of each Resource
  status := StateMonitor(statusInterval)

  // launch some Poller goroutines
  // channels allow main, Poller, and StateMonitor to communicate
  for i := 0; i < numPollers; i++ {
    go Poller(pending, complete, status)
  }

  // send some Resources to pending queue
  // take urls and pass info as Resource to pending channel
  // have to create another goroutine because channels send and receive synchronously
  // meaning send would be blocked until Poller was done
  go func() {
    for _, url := range urls {
      pending <- &Resource{url: url}
    }
  }()

  // when Poller is done with Resource, it sends it on the complete channel
  // For each Resource it starts a new goroutine calling Resource's Sleep method
  // using a new goroutine for each ensures that the sleeps can happen in parallel
  for r := range complete {
    go r.Sleep(pending)
  }
}
