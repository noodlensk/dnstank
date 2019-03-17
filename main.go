package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
)

// Runtime options
var (
	concurrency int
	verbose     bool
	resolver    string
	// check for empty response
	checkResponse        bool
	statsIntervalSeconds int
	// throttle between dns requests
	throttleMilliseconds int
)

func init() {
	log.SetFormatter(&log.JSONFormatter{})

	flag.IntVar(&concurrency, "concurrency", 50,
		"Number of threads")
	flag.IntVar(&statsIntervalSeconds, "p", 5,
		"Stats message printing interval in seconds")
	flag.IntVar(&throttleMilliseconds, "throttle", 0,
		"Throttle between requests in milliseconds")
	flag.BoolVar(&verbose, "v", false,
		"Verbose logging")
	flag.BoolVar(&checkResponse, "e", false,
		"Check response for empty result")
	flag.StringVar(&resolver, "r", "127.0.0.1:53",
		"Resolver to test against, by default will be used resolver from /etc/hosts")
	flag.Usage = func() {
		fmt.Printf(strings.Join([]string{
			"Send DNS requests as fast as possible (but with a throttle) to a given server and display the rate.",
			"",
			"Usage: dnstank [option ...] targetdomain [targetdomain [...] ]",
			"",
		}, "\n"))
		flag.PrintDefaults()
	}
}

func main() {
	flag.Parse()
	if verbose {
		log.SetLevel(log.DebugLevel)
	}
	// We need at least one target domain
	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	if !strings.Contains(resolver, ":") {
		// Automatically append the default port number if missing
		resolver = resolver + ":53"
	}

	// all remaining parameters are treated as domains to be used in round-robin in the threads
	targetDomains := make([]string, flag.NArg())
	for index, element := range flag.Args() {
		if element[len(element)-1] == '.' {
			targetDomains[index] = element
		} else {
			targetDomains[index] = element + "."
		}
	}
	log.WithFields(log.Fields{
		"resolver": resolver,
		"hosts":    strings.Join(targetDomains, ", "),
	}).Info("Started")
	ch := make(chan string)

	// Run concurrently
	for threadID := 0; threadID < concurrency; threadID++ {
		go func() {
			for host := range ch {
				res, err := resolveHost(host, resolver)
				if err != nil {
					log.WithField("error", err).Error("request failed")
					continue
				}
				if checkResponse && len(res.Answer) == 0 {
					log.Errorf("empty response: %v", res.String())
				}
				log.WithField("answer", res.String()).Debug("got response")
			}
		}()
	}

	// print statistic in background
	sentCnt := 0
	lastPrintedCnt := 0
	go func() {
		for {
			log.Infof("Sent %d requests, ~%.2f req/sec", sentCnt, float64((sentCnt-lastPrintedCnt)/statsIntervalSeconds))
			lastPrintedCnt = sentCnt
			time.Sleep(time.Second * time.Duration(statsIntervalSeconds))
		}
	}()

	// start sendind requests to channel
	throttle := time.Tick(time.Millisecond * time.Duration(throttleMilliseconds))
	for {
		for _, host := range targetDomains {
			<-throttle
			ch <- host
			sentCnt++
		}
	}
}

func resolveHost(host, resolver string) (*dns.Msg, error) {
	dnsconn, err := net.Dial("udp", resolver)
	if err != nil {
		return nil, err
	}
	co := &dns.Conn{Conn: dnsconn}
	defer co.Close()
	message := new(dns.Msg).SetQuestion(host, dns.TypeA)
	// Actually send the message and wait for answer
	co.WriteMsg(message)

	return co.ReadMsg()
}
