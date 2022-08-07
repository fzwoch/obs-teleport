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
	"encoding/json"
	"os"
	"sync"

	"github.com/schollz/peerdiscovery"
)

type Announcer struct {
	sync.WaitGroup
	ch chan struct{}
}

func (a *Announcer) StartAnnouncer(name string, port int, hasAudioAndVideo bool) {
	a.ch = make(chan struct{})

	a.Add(1)
	go func() {
		defer a.Done()

		if name == "" {
			var err error
			name, err = os.Hostname()
			if err != nil {
				name = "(None)"
			}
		}

		j := struct {
			Name          string
			Port          int
			AudioAndVideo bool
		}{
			Name:          name,
			Port:          port,
			AudioAndVideo: hasAudioAndVideo,
		}

		b, _ := json.Marshal(j)

		peerdiscovery.Discover(peerdiscovery.Settings{
			TimeLimit: -1,
			StopChan:  a.ch,
			Payload:   b,
		})
	}()
}

func (a *Announcer) StopAnnouncer() {
	close(a.ch)
	a.Wait()
}
