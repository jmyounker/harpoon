package main

// Warhead is a test application for
// It serves a few notable resources:
// - / Everybody's favorite rocket ASCII art.
// - /env The instance's environment variables as they have been set by Bazooka.
// - /metrics.json The instance's live health statistics, which Prometheus can
//                 consume and trigger events for.

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var (
	host         = flag.String("host", "0.0.0.0", "Bind address for HTTP server")
	port         = flag.Int("port", 8080, "Bind port for HTTP server")
	batchMode    = flag.Bool("batch-mode", false, "Do not start up HTTP server, just sleep and then terminate")
	boomMsg      = flag.String("msg", "Boom\n", "The string returned from HTTP Get /")
	exitCode     = flag.Int("exit-code", 0, "Exit from batch-mode with this code")
	leakInterval = flag.Duration("leak-interval", 0, "rate of memory leakage [0 is off]")
	oom          = flag.Bool("oom", false, "Terminate from batch mode with an OOM")
	runTime      = flag.Duration("run-time", 0*time.Second, "Time to run when in batch-mode")
)

func main() {
	go signalWatcher()
	if *batchMode {
		batchMain()
	} else {
		serverMain()
	}
}

func batchMain() {
	if *leakInterval != 0 {
		go leak(*leakInterval)
	}
	time.Sleep(*runTime)
	if *oom {
		// Attempt to allocate a lot of memory. This should still fail for a few more years.
		billion := 1000 * 1000 * 1000
		for i := 0; i < billion; i++ {
			b := make([]int, billion)
			// actually allocate the memory
			for j := 0; j < billion; j++ {
				b[j] = 0
			}
		}
	}
	os.Exit(*exitCode)
}

func serverMain() {
	// routes
	//
	// If you are using a single HTTP serving multiplexor or Go's
	// http.DefaultServeMux, you should try out Prometheus' wrappers
	// (prometheus.InstrumentHandler and prometheus.InstrumentHandlerFunc),
	// which will provide metric fan out for {handler name, HTTP method, and
	// HTTP status code} dimensions with histograms for request and response
	// size and for latency.
	http.HandleFunc("/", goBoom)
	http.HandleFunc("/fail", failHandler)
	http.HandleFunc("/env", envHandler)

	if *leakInterval != 0 {
		go leak(*leakInterval)
	}

	listen := fmt.Sprintf("%s:%d", *host, *port)
	log.Printf("listening on %s", listen)
	log.Fatal(http.ListenAndServe(listen, nil)) // Return a rc != 0 on failure
}

func envHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(strings.Join(os.Environ(), "\n")))

	log.Printf("%s %s", r.RemoteAddr, r.URL)
}

func goBoom(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(*boomMsg))

	log.Printf("%s %s", r.RemoteAddr, r.URL)
}

func failHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)

	log.Printf("%s %s", r.RemoteAddr, r.URL)

	os.Exit(1)
}

// signalWatcher gracefully handles operating system signal events, such as
// aborting the Dear Leader's rockets.
func signalWatcher() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals)

	for {
		signal := <-signals

		switch signal {
		case syscall.SIGINT, syscall.SIGTERM:
			log.Printf("received '%s' signal, exiting", signal)
			os.Exit(0)
		default:
			log.Printf("received '%s' signal, unhandled", signal)
		}
	}
}

func init() {
	log.SetOutput(os.Stdout) // All the logs belong to stdout
	flag.Parse()             // Parse command line arguments
}

func leak(interval time.Duration) {
	var src, dst []int

	for i := 0; i < 1024; i++ {
		src = append(src, i)
	}

	for _ = range time.Tick(interval) {
		// grow dst by 0...1024
		dst = append(dst, src[:rand.Intn(len(src))]...)

		slurp := func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if fi.IsDir() {
				return filepath.SkipDir
			}

			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()

			io.Copy(ioutil.Discard, f)
			return nil
		}

		// touch some pages from the stack
		filepath.Walk("/usr/include", slurp)
		// touch some pages from the application
		filepath.Walk("/srv/bazapp", slurp)
	}
}
