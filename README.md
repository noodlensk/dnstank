# dnstank

Simple stress test for dns server

## Usage

```bash
Send DNS requests as fast as possible (but with a throttle) to a given server and display the rate.

Usage: dnstank [option ...] targetdomain [targetdomain [...] ]
  -concurrency int
        Number of threads (default 50)
  -e    Check response for empty result
  -p int
        Stats message printing interval in seconds (default 5)
  -r string
        Resolver to test against, by default will be used resolver from /etc/resolv.conf
  -throttle int
        Throttle between requests in milliseconds
  -v    Verbose logging
```

## Installation

```BASH
make build
```
