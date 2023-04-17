package main

import (
	"bytes"
)

type Pool struct {
	c chan any
}

func NewPool(max int) *Pool {
	return &Pool{
		c: make(chan any, max),
	}
}

func (p *Pool) Get() (x any) {
	select {
	case x = <-p.c:
	default:
		x = &bytes.Buffer{}
	}
	return
}

func (p *Pool) Put(x any) {
	x.(*bytes.Buffer).Reset()
	select {
	case p.c <- x:
	default:
	}
}
