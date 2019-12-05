test:
	go test -race -v ./...

bench:
	go test -run="^$$" -bench=.

cover:
	go test $(shell go list ./... | grep -v /vendor/;) -cover -v

profile-cpu:
	go test -run="^$$" -bench=. -cpuprofile=c.p -benchtime=10s -v ./...

profile-mem:
	go test -run="^$$" -bench=. -memprofile=m.p -memprofilerate=1 -benchtime=10s -v ./...
