package main

import (
	"errors"
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

	statsIntervalSeconds int
	// throttle between dns requests
	throttleMilliseconds int

	// resolveMode could be "go-native" or "go-lib"
	// go-native should use glibc getaddr function
	resolveMode string

	// resolveType can be ip, ip4, ip6
	resolveType string
)

const (
	resolveModeNative = "go-native"
	resolveModeLib    = "go-lib"

	resolveTypeIP  = "ip"
	resolveTypeIP4 = "ip4"
	resolveTypeIP6 = "ip6"
)

var (
	errorTimeout  = errors.New("Timeout")
	errorNotFound = errors.New("Not found")
)

func init() {
	log.SetFormatter(&log.JSONFormatter{})

	flag.IntVar(&concurrency, "concurrency", 50,
		"Number of threads")
	flag.IntVar(&statsIntervalSeconds, "p", 5,
		"Stats message printing interval in seconds")
	flag.IntVar(&throttleMilliseconds, "throttle", 1,
		"Throttle between requests in milliseconds")
	flag.BoolVar(&verbose, "v", false,
		"Verbose logging")
	flag.StringVar(&resolver, "r", "",
		"Resolver to test against, by default will be used resolver from /etc/resolv.conf")
	flag.StringVar(&resolveMode, "m", "go-lib",
		"Resolving mode: can be 'go-native' or 'go-lib'[default]")
	flag.StringVar(&resolveType, "t", "ip",
		"Resolving type: can be ip, ip4, ip6")
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

	if resolver == "" {
		config, err := dns.ClientConfigFromFile("/etc/resolv.conf")
		if err != nil {
			log.Fatalf("failed to read /etc/resolv.conf: %+v", err)
		}
		resolver = config.Servers[0] + ":" + config.Port
	}

	if !strings.Contains(resolver, ":") {
		// Automatically append the default port number if missing
		resolver = resolver + ":53"
	}

	// check for modes
	switch resolveMode {
	case resolveModeLib, resolveModeNative:
	default:
		log.Fatalf("unknown mode %s", resolveMode)
	}
	// check for resolve type
	switch resolveType {
	case resolveTypeIP, resolveTypeIP4, resolveTypeIP6:
	default:
		log.Fatalf("unknown type %s", resolveType)
	}

	// all remaining parameters are treated as domains to be used in round-robin in the threads
	targetDomains := make([]string, flag.NArg())
	for index, element := range flag.Args() {
		targetDomains[index] = element
	}
	log.WithFields(log.Fields{
		"resolver": resolver,
		"mode":     resolveMode,
		"type":     resolveType,
		"hosts":    strings.Join(targetDomains, ", "),
	}).Info("Started")
	ch := make(chan string)
	errorCh := make(chan error)
	sentCnt := 0
	notFoundCnt := 0
	timeoutCnt := 0
	errorCnt := 0
	go func() {
		for err := range errorCh {
			switch err {
			case errorNotFound:
				notFoundCnt++
			case errorTimeout:
				timeoutCnt++
			default:
				errorCnt++
			}
		}
	}()
	// Run concurrently
	for threadID := 0; threadID < concurrency; threadID++ {
		go func() {
			for host := range ch {
				var err error
				if resolveMode == resolveModeLib {
					err = resolveHostLib(host, resolver)
				}
				if resolveMode == resolveModeNative {
					err = resolveHostNative(host)
				}
				if err != nil {
					errorCh <- err
				}
			}
		}()
	}

	// print statistic in background
	lastPrintedCnt := 0
	go func() {
		for {
			log.Infof("Total %d, not found: %d, timeout: %d, errors: %d, ~%.2f req/sec", sentCnt, notFoundCnt, timeoutCnt, errorCnt, float64((sentCnt-lastPrintedCnt))/float64(statsIntervalSeconds))
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

func resolveHostLib(host, resolver string) error {
	client := &dns.Client{
		Net:     "udp",
		Timeout: time.Second * 5,
	}
	co, err := client.Dial(resolver)
	if err != nil {
		return err
	}
	defer co.Close()
	if err := co.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	if host[len(host)-1] != '.' {
		host = host + "."
	}
	message := new(dns.Msg).SetQuestion(host, dns.TypeA)
	// Actually send the message and wait for answer
	co.WriteMsg(message)
	res, err := co.ReadMsg()
	if err != nil {
		return err
	}
	if len(res.Answer) == 0 {
		return errorNotFound
	}
	return nil
}

func resolveHostNative(host string) error {
	res, err := net.ResolveIPAddr(resolveType, host)
	if err != nil {
		if strings.Contains(err.Error(), "no such host") {
			return errorNotFound
		}
		return err
	}
	if res.String() == "" {
		return errorNotFound
	}
	return nil
}
