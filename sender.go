//
// obs-teleport. OBS Studio plugin.
// Copyright (C) 2021-2022 Florian Zwoch <fzwoch@gmail.com>
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
	"net"
	"sync"
)

type Sender struct {
	sync.Mutex
	sync.WaitGroup
	conns map[net.Conn]any
}

func (q *Sender) AddConnection(c net.Conn) {
	q.Lock()
	defer q.Unlock()

	if q.conns == nil {
		q.conns = make(map[net.Conn]any)
	}

	q.conns[c] = nil
}

func (q *Sender) SendData(b []byte) {
	q.Lock()
	defer q.Unlock()

	for c := range q.conns {
		q.Add(1)

		go func(c net.Conn) {
			defer q.Done()

			_, err := c.Write(b)
			if err != nil {
				c.Close()
				q.Lock()
				delete(q.conns, c)
				q.Unlock()
			}
		}(c)
	}
}

func (q *Sender) CloseAll() {
	q.Lock()

	for c := range q.conns {
		c.Close()
	}

	q.conns = make(map[net.Conn]any)

	q.Unlock()
	q.Wait()
}
