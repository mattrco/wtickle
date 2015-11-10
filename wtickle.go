// wtickle reads a list of URLs from STDIN and then randomly gets
// those URLs for some period of time with controllable concurrency.

package main

import (
	"bufio"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var client http.Client

type responseWithError struct {
	resp *http.Response
	err  error
}

// Reads URLs from work channel and performs the request and sends
// detail of response down result channel
func worker(wg *sync.WaitGroup, work chan string,
	result chan responseWithError, hdr, val string) {
	for url := range work {
		request, err := http.NewRequest("GET", url, nil)
		if err != nil {
			fmt.Printf("Error creating request: %s", err)
			break
		}
		if hdr != "" {
			request.Header.Add(hdr, val)
		}
		resp, err := client.Do(request)
		result <- responseWithError{resp, err}
	}

	wg.Done()
}

// Just reads from the result channel and outputs values and writes
// the log
func reader(result chan responseWithError, log *os.File) {
	for re := range result {

		// Outputs
		//
		// . for 200 OK
		// e for internal error in this program
		// First character of status code (e.g. 3, 4, 5)

		output := ""
		tolog := []string{re.resp.Request.URL.String()}

		switch {
		case re.err != nil:
			output = "e"
			tolog = append(tolog, fmt.Sprintf("Error %s", re.err))
		case re.resp.StatusCode == http.StatusOK:
			output = "."
			tolog = append(tolog, fmt.Sprintf("%s", re.resp.Status))
			for k, v := range re.resp.Header {
				tolog = append(tolog, fmt.Sprintf("%s: %s", k, v))
			}
		default:
			output = re.resp.Status[0:1]
			tolog = append(tolog, fmt.Sprintf("%s", re.resp))
		}
		re.resp.Body.Close()
		fmt.Print(output)
		if log != nil {
			tolog = append(tolog, "", "")
			log.WriteString(strings.Join(tolog, "\n"))
		}
	}
}

// Outputs random URLs from the set of URLs until the duration
// runs out
func writer(work chan string, duration time.Duration, urls []string) {

	// Note use of nil channel here so that if duration is infinite
	// this function never returns

	var terminator <-chan time.Time
	if duration > 0 {
		terminator = time.After(duration)
	}

	for {
		select {
		case <-terminator:
			close(work)
			return

		case work <- urls[rand.Intn(len(urls))]:
		}
	}
}

func main() {
	par := flag.Int("par", 10, "Number of parallel requests")
	header := flag.String("header", "", "Optional HTTP header to insert")
	duration := flag.Duration("duration", 0, "Optional duration; 0 = forever")
	log := flag.String("log", "", "log file to write detailed output to")
	flag.Parse()

	var hdr, val string
	if *header != "" {
		parts := strings.SplitN(*header, " ", 2)
		if len(parts) != 2 {
			fmt.Printf("Error: bad header %s\n", *header)
			return
		}
		hdr = parts[0]
		val = parts[1]
	}

	var logger *os.File
	if *log != "" {
		var err error
		logger, err = os.Create(*log)
		if err != nil {
			fmt.Printf("Error creating log file %s: %s\n", *log, err)
			return
		}
	}

	var urls []string

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		url := scanner.Text()
		if url != "" {
			urls = append(urls, url)
		}
	}

	if len(urls) == 0 {
		fmt.Printf("Error: no URLs found")
		return
	}

	work := make(chan string)
	result := make(chan responseWithError)

	var wg sync.WaitGroup
	for i := 0; i < *par; i++ {
		wg.Add(1)
		go worker(&wg, work, result, hdr, val)
	}

	go writer(work, *duration, urls)
	go reader(result, logger)

	wg.Wait()
	close(result)
	if logger != nil {
		logger.Close()
	}

	fmt.Printf("\n")
}