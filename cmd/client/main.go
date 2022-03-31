package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/andrewstucki/light/tunnel"
)

func main() {
	if err := tunnel.Connect(context.Background(), tunnel.Config{
		Address: "localhost",
		ID:      "foo",
		Handler: http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			response.WriteHeader(http.StatusTeapot)
			response.Write([]byte("hi\n"))
		}),
		Port: 8080,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
