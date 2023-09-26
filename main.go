package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gomodule/redigo/redis"
)

var (
	redisServer = flag.String("redisServer", "localhost:6379", "")
	port        = flag.Int("port", 4000, "")
)

func main() {
	flag.Parse()

	// Initialize a connection pool and assign it to the pool global
	// variable.
	pool = &redis.Pool{
		MaxIdle:     10,
		IdleTimeout: 240 * time.Second,

		Dial: func() (redis.Conn, error) {
			return redis.Dial("tcp", *redisServer)
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/album", showAlbum)
	mux.HandleFunc("/like", addLike)
	mux.HandleFunc("/popular", listPopular)

	addr := fmt.Sprintf(":%d", *port)
	log.Println("Serving on " + addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal("serve error:", err)
	}
}

func showAlbum(w http.ResponseWriter, r *http.Request) {
	// Unless the request is using the GET method, return a 405 'Method
	// Not Allowed' response.
	if http.MethodGet != r.Method {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	// Retrieve the id from the request URL query string.
	// If there is no id key in the query string or is not valid integer
	// then retrieveID() will return an empty string.
	// We check for this, returning a 400 Bad Request response.
	id := retrieveID(r)
	if id == "" {
		http.Error(w, http.StatusText(400), 400)
		return
	}

	// Call the FindAlbum() function passing in the user-provided id.
	// If there's no matching album found, return a 404 Not Found
	// response. In the event of any other errors, return a 500
	// Internal Server Error response.
	ab, err := FindAlbum(id)
	switch err {
	default:
		http.Error(w, http.StatusText(500), 500)
		return
	case ErrNoAlbum:
		http.NotFound(w, r)
		return
	case nil:
	}

	// Write the album details as plain text to the client.
	fmt.Fprintf(w, "%s\n", ab)
}

func retrieveID(r *http.Request) string {
	var id string
	switch r.Method {
	case http.MethodGet:
		// Retrieve the id from the request URL query string. If there is
		// no id key in the query string then Get() will return an empty string.
		id = r.URL.Query().Get("id")
	case http.MethodPost:
		// Retrieve the id from the POST request body. If there is no
		// parameter named "id" in the request body then PostFormValue()
		// will return an empty string.
		id = r.PostFormValue("id")
	}

	if id != "" {
		// Validate that the id is a valid integer by trying to convert it.
		if _, err := strconv.Atoi(id); err == nil {
			return id
		}
	}
	return ""
}

func addLike(w http.ResponseWriter, r *http.Request) {
	// Unless the request is using the POST method, return a 405
	// Method Not Allowed response.
	if http.MethodPost != r.Method {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	// Retrieve the id from the POST request body. If there is no
	// parameter named "id" in the request body or is not valid integer
	// then retrieveID() will return an empty string.
	// We check for this, returning a 400 Bad Request response.
	id := retrieveID(r)
	if id == "" {
		http.Error(w, http.StatusText(400), 400)
		return
	}

	// Call the IncrementLikes() function passing in the user-provided
	// id. If there's no album found with that id, return a 404 Not
	// Found response. In the event of any other errors, return a 500
	// Internal Server Error response.
	err := IncrementLikes(id)
	switch err {
	default:
		http.Error(w, http.StatusText(500), 500)
		return
	case ErrNoAlbum:
		http.NotFound(w, r)
		return
	case nil:
	}

	// Redirect the client to the GET /album route, so they can see the
	// impact their like has had.
	http.Redirect(w, r, "/album?id="+id, http.StatusSeeOther)
}

func listPopular(w http.ResponseWriter, r *http.Request) {
	// Unless the request is using the GET method, return a 405 'Method Not
	// Allowed' response.
	if http.MethodGet != r.Method {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	// Call the FindTopThree() function, returning a return a 500 Internal
	// Server Error response if there's any error.
	albums, err := FindTopThree()
	if err != nil {
		http.Error(w, http.StatusText(500), 500)
		return
	}

	// Loop through the 3 albums, writing the details as a plain text list
	// to the client.
	for i, ab := range albums {
		fmt.Fprintf(w, "%d) %s\n", i+1, ab)
	}
}
