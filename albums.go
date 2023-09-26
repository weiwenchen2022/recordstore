package main

import (
	"errors"
	"fmt"
	"log"

	"github.com/gomodule/redigo/redis"
)

// Declare a pool variable to hold the pool of Redis connections.
var pool *redis.Pool

var ErrNoAlbum = errors.New("no album found")

// Album defines a custom struct to hold album data.
type Album struct {
	ID     int     `redis:"id"`
	Title  string  `redis:"title"`
	Artist string  `redis:"artist"`
	Price  float64 `redis:"price"`
	Likes  int     `redis:"likes"`
}

func (a Album) String() string {
	return fmt.Sprintf("%s by %s: £%.2f [%d likes]",
		a.Title, a.Artist, a.Price, a.Likes)
}

func FindAlbum(id string) (*Album, error) {
	// Use the connection pool's Get() method to fetch a single Redis
	// connection from the pool.
	conn := pool.Get()

	// Importantly, use defer and the connection's Close() method to
	// ensure that the connection is always returned to the pool before
	// FindAlbum() exits.
	defer conn.Close()

	// Fetch the details of a specific album. If no album is found
	// the given id, the []interface{} slice returned by redis.Values
	// will have a length of zero. So check for this and return an
	// ErrNoAlbum error as necessary.
	values, err := redis.Values(conn.Do("HGETALL", "album:"+id))
	switch {
	case err != nil:
		return nil, err
	case len(values) == 0:
		return nil, ErrNoAlbum
	}

	var album Album
	if err := redis.ScanStruct(values, &album); err != nil {
		return nil, err
	}
	return &album, nil
}

var incrementLikesScript = redis.NewScript(1, `
	local exists = redis.call('EXISTS', 'album:' .. KEYS[1])
	if exists == 0 then
		return -1
	end
	
	redis.call('HINCRBY', 'album:' ..  KEYS[1], 'likes', 1)
	return redis.call('ZINCRBY', 'likes', 1, KEYS[1])
`)

func IncrementLikes(id string) error {
	conn := pool.Get()
	defer conn.Close()

	likes, err := redis.Int(incrementLikesScript.Do(conn, id))
	if err != nil {
		return err
	} else if likes < 0 {
		return ErrNoAlbum
	}
	return nil
}

func FindTopThree() ([]*Album, error) {
	conn := pool.Get()
	defer conn.Close()

	// Begin an infinite loop. In a real application, you might want to
	// limit this to a set number of attempts, and return an error if
	// the transaction doesn't successfully complete within those
	// attempts.
	for {
		// Instruct Redis to watch the likes sorted set for any changes.
		_, err := conn.Do("WATCH", "likes")
		if err != nil {
			return nil, err
		}

		// Use the ZREVRANGE command to fetch the album ids with the
		// highest score (i.e. most likes) from our 'likes' sorted set.
		// The ZREVRANGE start and stop values are zero-based indexes,
		// so we use 0 and 2 respectively to limit the reply to the top
		// three. Because ZREVRANGE returns an array response, we use
		// the Strings() helper function to convert the reply into a
		// []string.
		ids, err := redis.Strings(conn.Do("ZREVRANGE", "likes", 0, 2))
		if err != nil {
			return nil, err
		}

		// Use the MULTI command to inform Redis that we are starting
		// a new transaction.
		err = conn.Send("MULTI")
		if err != nil {
			return nil, err
		}

		// Loop through the ids returned by ZREVRANGE, queuing HGETALL
		// commands to fetch the individual album details.
		for _, id := range ids {
			err := conn.Send("HGETALL", "album:"+id)
			if err != nil {
				return nil, err
			}
		}

		// Execute the transaction. Importantly, use the redis.ErrNil
		// type to check whether the reply from EXEC was nil or not. If
		// it is nil it means that another client changed the WATCHed
		// likes sorted set, so we use the continue command to re-run
		// the loop.
		replies, err := redis.Values(conn.Do("EXEC"))
		switch err {
		default:
			return nil, err
		case redis.ErrNil:
			log.Print("trying again")
			continue
		case nil:
		}

		// Create a new slice to store the album details.
		albums := make([]*Album, len(ids))

		// Iterate through the array of response objects, using the
		// ScanStruct() function to assign the data to Album structs.
		for i, reply := range replies {
			var album Album
			err = redis.ScanStruct(reply.([]interface{}), &album)
			if err != nil {
				return nil, err
			}
			albums[i] = &album
		}
		return albums, nil
	}
}
