package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"syscall"
	"time"

	"golang.org/x/net/proxy"
)

const (
	minInterval    = 60
	maxInterval    = 900
	defaultUrl     = "https://www.google.com/"
	requestTimeout = 30
)

func prepareProxyTransport(proxyUrl string) (*http.Transport, error) {
	var dialer proxy.Dialer

	dialer = proxy.Direct

	if proxyUrl != "" {
		u, err := url.Parse(proxyUrl)
		if err != nil {
			return nil, err
		}

		dialer, err = proxy.FromURL(u, dialer)
		if err != nil {
			return nil, err
		}
	}

	transport := &http.Transport{Dial: dialer.Dial}
	return transport, nil
}

func fetchTime(proxyUrl string, targetUrl string) (parsed time.Time, err error) {
	transport, err := prepareProxyTransport(proxyUrl)
	if err != nil {
		return
	}

	client := &http.Client{
		Transport: transport,
		Timeout: requestTimeout * time.Second,
	}

	log.Printf("Start request to %q", targetUrl)

	resp, err := client.Get(targetUrl)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	dateHeader := resp.Header.Get("Date")
	parsed, err = time.Parse("Mon, 02 Jan 2006 15:04:05 MST", dateHeader)
	if err != nil {
		return
	}

	minTime := time.Date(2016, 7, 1, 0, 0, 0, 0, time.UTC)
	if parsed.Before(minTime) {
		err = errors.New("Timestamp from server is below minimum.")
		return
	}

	return
}

func main() {
	var printOnly bool
	var skipSet bool
	var proxyUrl string
	var targetUrl string
	flag.BoolVar(&printOnly, "printonly", false, "Print the time and immediately exit")
	flag.BoolVar(&skipSet, "skipset", false, "Don't try to set the system clock")
	flag.StringVar(&proxyUrl, "proxy", "", "URL of proxy used to access the server")
	flag.StringVar(&targetUrl, "url", defaultUrl, "URL to an HTTP server with an accurate Date header")
	flag.Parse()

	if printOnly {
		fetched, err := fetchTime(proxyUrl, targetUrl)
		if err != nil {
			log.Fatalf("%v", err)
		}

		fmt.Printf("%s\n", fetched.UTC())
		os.Exit(0)
	}

	// seed the random number generated used for sleep intervals
	rand.Seed(time.Now().UnixNano() * int64(os.Getpid()))

	for {
		fetched, err := fetchTime(proxyUrl, targetUrl)
		if err == nil {
			now := time.Now()
			offset := fetched.Sub(now)

			log.Printf("Remote time: %s", fetched.UTC())
			log.Printf("System time: %s", now.UTC())
			log.Printf("Remote offset from system clock: %v", offset)

			if !skipSet {
				state, err := syscall.Adjtimex(&syscall.Timex{
					Modes:  1, // ADJ_OFFSET = 1
					Offset: int64(offset / time.Microsecond),
				})
				if err != nil {
					log.Printf("Failed to set system clock: %v", err)
				}
				if state != 0 {
					log.Printf("Return value of adjtime call is nonzero: %v", state)
				}
			}
		} else {
			log.Printf("Error fetching time: %v", err)
		}

		sleepSecs := rand.Intn(maxInterval-minInterval) + minInterval
		interval := time.Duration(sleepSecs) * time.Second
		log.Printf("Sleeping for %v", interval)
		time.Sleep(interval)
	}
}
