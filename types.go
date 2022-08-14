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
	"bytes"
	"encoding/binary"
	"image"

	"github.com/pixiv/go-libjpeg/jpeg"
)

type AnnouncePayload struct {
	Name          string
	Port          int
	AudioAndVideo bool
}

type Header struct {
	Type      [4]byte
	Timestamp uint64
	Size      int32
}

type ImageHeader struct {
	ColorMatrix   [16]float32
	ColorRangeMin [3]float32
	ColorRangeMax [3]float32
}

type WaveHeader struct {
	Format     int32
	SampleRate int32
	Speakers   int32
	Frames     int32
}

type Packet struct {
	Header         Header
	ImageHeader    ImageHeader
	WaveHeader     WaveHeader
	Buffer         []byte
	IsAudio        bool
	DoneProcessing bool
	Quality        int
	Image          image.Image
}

func (p *Packet) ToJPEG() {
	b := bytes.Buffer{}

	jpeg.Encode(&b, p.Image, &jpeg.EncoderOptions{
		Quality: p.Quality,
	})

	p.Header.Type = [4]byte{'J', 'P', 'E', 'G'}
	p.Header.Size = int32(b.Len())

	h := bytes.Buffer{}

	binary.Write(&h, binary.LittleEndian, &p.Header)
	binary.Write(&h, binary.LittleEndian, &p.ImageHeader)

	p.Buffer = append(h.Bytes(), b.Bytes()...)
}
