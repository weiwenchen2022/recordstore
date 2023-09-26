package main

import (
	"fmt"
	"log"
	"strconv"
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/google/go-cmp/cmp"
)

const KeyPattern = "album:%d"

func init() {
	pool = &redis.Pool{
		MaxIdle:     10,
		IdleTimeout: 240 * time.Second,

		Dial: func() (redis.Conn, error) {
			return redis.Dial("tcp", "localhost:6379")
		},
	}
}

var albums = []*Album{
	{ID: 1, Title: "Electric Ladyland", Artist: "Jimi Hendrix", Price: 4.95, Likes: 8},
	{ID: 2, Title: "Back in Black", Artist: "AC/DC", Price: 5.95, Likes: 3},
	{ID: 3, Title: "Rumours", Artist: "Fleetwood Mac", Price: 7.95, Likes: 12},
	{ID: 4, Title: "Nevermind", Artist: "Nirvana", Price: 5.95, Likes: 8},
}

func TestHMSET(t *testing.T) {
	conn := pool.Get()
	defer conn.Close()

	for _, ab := range albums {
		err := conn.Send("MULTI")
		if err != nil {
			t.Fatal(err)
		}

		err = conn.Send("HMSET", redis.Args{}.Add(fmt.Sprintf(KeyPattern, ab.ID)).AddFlat(ab)...)
		if err != nil {
			log.Fatal(err)
		}
		err = conn.Send("ZADD", redis.Args{}.Add("likes").AddFlat([]int{ab.Likes, ab.ID})...)
		if err != nil {
			log.Fatal(err)
		}

		_, err = conn.Do("EXEC")
		if err != nil {
			t.Fatal(err)
		}

		t.Log(ab.Title + " added!")
	}

	results := make([]*Album, len(albums))
	for i, ab := range albums {
		values, err := redis.Values(conn.Do("HGETALL", fmt.Sprintf(KeyPattern, ab.ID)))
		if err != nil {
			t.Fatal(err)
		}

		var album Album
		if err := redis.ScanStruct(values, &album); err != nil {
			t.Fatal(err)
		}
		results[i] = &album
	}

	if !cmp.Equal(albums, results) {
		t.Error(cmp.Diff(albums, results))
	}
}

func TestFindAlbum(t *testing.T) {
	ab, err := FindAlbum("2")
	if err != nil {
		t.Fatal(err)
	}

	want := albums[1]
	if !cmp.Equal(want, ab) {
		t.Error(cmp.Diff(want, ab))
	}
}

func TestIncrementLikes(t *testing.T) {
	tests := []struct {
		id      int
		wantErr error
	}{
		{id: 2},
		{id: 5, wantErr: ErrNoAlbum},
	}

	for _, tt := range tests {
		id := strconv.Itoa(tt.id)
		ab, err := FindAlbum(id)
		if (err != nil) != (tt.wantErr != nil) {
			t.Errorf("unexpected error: %v, wantErr: %v", err, tt.wantErr)
			continue
		}

		err = IncrementLikes(id)
		if err != nil != (tt.wantErr != nil) ||
			(tt.wantErr != nil && tt.wantErr != err) {
			t.Errorf("unexpected error: %v, wantErr: %v", err, tt.wantErr)
			continue
		}

		if tt.wantErr != nil {
			continue
		}

		newAb, err := FindAlbum(id)
		if err != nil {
			t.Error(err)
			continue
		}
		if newAb.Likes-ab.Likes != 1 {
			t.Errorf("likes got: %d, want: %d", newAb.Likes, ab.Likes+1)
		}
	}
}

func TestFindTopThree(t *testing.T) {
	albums, err := FindTopThree()
	if err != nil {
		t.Fatal(err)
	}

	for _, ab := range albums {
		t.Logf("%+v\n", ab)
	}
}
