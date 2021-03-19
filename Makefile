build:
	go mod tidy &&\
	go build
release:
	mkdir release && \
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o ./release/dnstank_linux && \
	zip --junk-paths  dnstank ./release/dnstank_linux &&\
	rm -rf release