//
// obs-teleport. OBS Studio plugin.
// Copyright (C) 2021-2024 Florian Zwoch <fzwoch@gmail.com>
//
// This file is part of obs-teleport.
//
// obs-teleport is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 2 of the License, or
// (at your option) any later version.
//
// obs-teleport is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with obs-teleport. If not, see <http://www.gnu.org/licenses/>.
//

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
