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

type Buffer struct {
	Header         Header
	ImageHeader    ImageHeader
	WaveHeader     WaveHeader
	Buffer         []byte
	DoneProcessing bool
	Quality        int
	Image          image.Image
}

func (b *Buffer) CreateJPEG() {
	p := bytes.Buffer{}

	jpeg.Encode(&p, b.Image, &jpeg.EncoderOptions{
		Quality: b.Quality,
	})

	head := bytes.Buffer{}

	binary.Write(&head, binary.LittleEndian, &b.Header)

	//binary.Write(&head, binary.LittleEndian, &image_header)

	b.Buffer = append(head.Bytes(), p.Bytes()...)
}

func (b *Buffer) CreateWAVE() {

}
