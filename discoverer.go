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
	"sync"
	"time"

	"github.com/schollz/peerdiscovery"
)

type Discoverer struct {
	sync.WaitGroup
	ch chan struct{}
}

func (d *Discoverer) StartDiscoverer(services map[string]Peer, h sync.Locker) {
	d.ch = make(chan struct{})

	d.Add(1)
	go func() {
		defer d.Done()

		peerdiscovery.Discover(peerdiscovery.Settings{
			TimeLimit:        -1,
			StopChan:         d.ch,
			AllowSelf:        true,
			DisableBroadcast: true,
			Notify: func(d peerdiscovery.Discovered) {
				j := AnnouncePayload{}

				err := json.Unmarshal(d.Payload, &j)
				if err != nil {
					return
				}

				h.Lock()
				services[j.Name+":"+d.Address] = Peer{
					Payload: j,
					Address: d.Address,
					Time:    time.Now().Add(5 * time.Second),
				}
				h.Unlock()
			},
		})
	}()
}

func (d *Discoverer) StopDiscoverer() {
	close(d.ch)
	d.Wait()
}
