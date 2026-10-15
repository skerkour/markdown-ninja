package singleinstance

import "log"

// ExampleNew is an example how to use singleinstance
func ExampleNew() {
	// create a new lockfile in /var/lock/filename
	one := New("filename", WithLockPath("/tmp"))

	// lock and defer unlocking
	if err := one.Lock(); err != nil {
		log.Fatal(err)
	}

	// run

	if err := one.Unlock(); err != nil {
		log.Println(err)
	}
}
