//
// obs-teleport. OBS Studio plugin.
// Copyright (C) 2021-2023 Florian Zwoch <fzwoch@gmail.com>
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

//
// #include <obs-module.h>
//
// extern void blog_string(const int log_level, const char* string);
//
import "C"
import (
	"net"
	"strconv"
	"sync"
	"unsafe"
)

type Sender struct {
	sync.Mutex
	sync.WaitGroup
	conns map[net.Conn]chan []byte
}

func (s *Sender) SenderAdd(c net.Conn) {
	s.Lock()
	defer s.Unlock()

	if s.conns == nil {
		s.conns = make(map[net.Conn]chan []byte)
	}

	ch := make(chan []byte, 1000)
	s.conns[c] = ch

	s.Add(1)
	go func() {
		defer s.Done()
		defer c.Close()

		for b := range ch {
			_, err := c.Write(b)
			if err != nil {
				s.Lock()
				close(ch)
				delete(s.conns, c)
				s.Unlock()
				break
			}
		}
	}()
}

func (s *Sender) SenderGetNumConns() int {
	s.Lock()
	defer s.Unlock()

	return len(s.conns)
}

func (s *Sender) SenderSend(b []byte) {
	s.Lock()
	defer s.Unlock()

	for c, ch := range s.conns {
		if len(ch) > 800 {
			tmp := C.CString("send queue exceeded [" + c.RemoteAddr().String() + "] " + strconv.Itoa(len(ch)))
			C.blog_string(C.LOG_WARNING, tmp)
			C.free(unsafe.Pointer(tmp))
			continue
		} else if len(ch) > 100 {
			tmp := C.CString("send queue high [" + c.RemoteAddr().String() + "] " + strconv.Itoa(len(ch)))
			C.blog_string(C.LOG_WARNING, tmp)
			C.free(unsafe.Pointer(tmp))
		}

		ch <- b
	}
}

func (s *Sender) SenderClose() {
	s.Lock()

	for _, ch := range s.conns {
		close(ch)
	}

	s.conns = nil

	s.Unlock()
	s.Wait()
}
