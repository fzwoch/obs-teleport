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

//
// #include <obs-module.h>
//
// extern void blog_string(const int log_level, const char* string);
//
import "C"
import (
	"net"
	"sync"
	"unsafe"
)

type Sender struct {
	sync.Mutex
	sync.WaitGroup
	conns map[net.Conn]any
	ch    chan []byte
}

func (s *Sender) SenderAdd(c net.Conn) {
	s.Lock()
	defer s.Unlock()

	if s.conns == nil {
		s.conns = make(map[net.Conn]any)
	}

	s.conns[c] = nil
}

func (s *Sender) SenderGetNumConns() int {
	s.Lock()
	defer s.Unlock()

	return len(s.conns)
}

func (s *Sender) SenderSend(b []byte) {
	s.Lock()
	defer s.Unlock()

	if s.ch == nil {
		s.ch = make(chan []byte, 1000)

		s.Add(1)
		go func() {
			defer s.Done()

			for b := range s.ch {
				s.Lock()
				for c := range s.conns {
					s.Add(1)
					go func(c net.Conn) {
						defer s.Done()

						_, err := c.Write(b)
						if err != nil {
							c.Close()
							s.Lock()
							delete(s.conns, c)
							s.Unlock()
						}
					}(c)
				}
				s.Unlock()
			}

			s.ch = nil
		}()
	}

	if len(s.ch) > 800 {
		tmp := C.CString("Send Queue exceeded")
		C.blog_string(C.LOG_WARNING, tmp)
		C.free(unsafe.Pointer(tmp))
		return
	} else if len(s.ch) > 100 {
		tmp := C.CString("Send Queue high")
		C.blog_string(C.LOG_WARNING, tmp)
		C.free(unsafe.Pointer(tmp))
	}

	s.ch <- b
}

func (s *Sender) SenderClose() {
	s.Lock()

	if s.ch != nil {
		close(s.ch)
	}

	for c := range s.conns {
		c.Close()
	}

	s.conns = make(map[net.Conn]any)

	s.Unlock()
	s.Wait()
}
