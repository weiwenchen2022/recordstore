package main

import (
	"errors"
	"fmt"

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
		return redis.error_reply('no album found')
	end
	
	redis.call('HINCRBY', 'album:' ..  KEYS[1], 'likes', 1)
	return redis.call('ZINCRBY', 'likes', 1, KEYS[1])
`)

func IncrementLikes(id string) error {
	conn := pool.Get()
	defer conn.Close()

	_, err := redis.Int(incrementLikesScript.Do(conn, id))
	if err != nil {
		if ErrNoAlbum.Error() == err.Error() {
			err = ErrNoAlbum
		}
		return err
	}
	return nil
}

var findTopThreeScript = redis.NewScript(0, `
	local ids = redis.call('ZREVRANGE', 'likes', 0, 2)
	local albums = {}
	for i = 1, #ids do
		table.insert(albums, redis.call('HGETALL', 'album:' .. ids[i]))
	end
	return albums
`)

func FindTopThree() ([]*Album, error) {
	conn := pool.Get()
	defer conn.Close()

	replies, err := redis.Values(findTopThreeScript.Do(conn))
	if err != nil {
		return nil, err
	}

	albums := make([]*Album, len(replies))
	for i, reply := range replies {
		var album Album
		err := redis.ScanStruct(reply.([]any), &album)
		if err != nil {
			return nil, err
		}
		albums[i] = &album
	}
	return albums, nil
}
