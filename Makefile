dev:
	go run main.go

kill:
	sudo lsof -t -i:8080 | xargs kill -9
