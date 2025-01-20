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

//
// #cgo LDFLAGS: -lturbojpeg
//
// #include <obs-module.h>
// #include <turbojpeg.h>
//
import "C"
import (
	"bytes"
	"encoding/binary"
	"image"
	"runtime"
	"unsafe"
)

type Packet struct {
	Header         Header
	ImageHeader    ImageHeader
	WaveHeader     WaveHeader
	Buffer         []byte
	IsAudio        bool
	DoneProcessing bool
	Quality        int
	Image          image.Image
	ImageBuffer    *bytes.Buffer
}

func (p *Packet) ToJPEG(pool *Pool) {
	ctx := C.tj3Init(C.TJINIT_COMPRESS)
	defer C.tj3Destroy(ctx)

	C.tj3Set(ctx, C.TJPARAM_NOREALLOC, 1)
	C.tj3Set(ctx, C.TJPARAM_QUALITY, C.int(p.Quality))

	var (
		buf         []byte
		subsampling C.int
		tmp         *C.uchar
		size        C.size_t
		pinner      runtime.Pinner
	)

	switch p.Image.(type) {
	case *image.YCbCr:
		img := p.Image.(*image.YCbCr)

		switch img.SubsampleRatio {
		case image.YCbCrSubsampleRatio420:
			subsampling = C.TJSAMP_420
		case image.YCbCrSubsampleRatio422:
			subsampling = C.TJSAMP_422
		case image.YCbCrSubsampleRatio444:
			subsampling = C.TJSAMP_444
		default:
			panic("invalid subsampling")
		}

		C.tj3Set(ctx, C.TJPARAM_SUBSAMP, subsampling)

		s := C.tj3JPEGBufSize(C.int(img.Rect.Dx()), C.int(img.Rect.Dy()), subsampling)

		buf = make([]byte, int(s))
		tmp = (*C.uchar)(&buf[0])

		pinner.Pin(tmp)
		C.tj3CompressFromYUV8(ctx, (*C.uchar)(&img.Y[0]), C.int(img.Rect.Dx()), 1, C.int(img.Rect.Dy()), &tmp, &size)
		pinner.Unpin()
	case *image.RGBA:
		img := p.Image.(*image.RGBA)

		C.tj3Set(ctx, C.TJPARAM_SUBSAMP, C.TJSAMP_444)
		C.tj3Set(ctx, C.TJPARAM_COLORSPACE, C.TJCS_RGB)

		s := C.tj3JPEGBufSize(C.int(img.Rect.Dx()), C.int(img.Rect.Dy()), C.TJSAMP_444)

		buf = make([]byte, int(s))
		tmp = (*C.uchar)(&buf[0])

		pinner.Pin(tmp)
		C.tj3Compress8(ctx, (*C.uchar)(&img.Pix[0]), C.int(img.Rect.Dx()), 0, C.int(img.Rect.Dy()), C.TJPF_BGRX, &tmp, &size)
		pinner.Unpin()
	default:
		panic("invalid image type")
	}

	buf = buf[:int(size)]

	p.Image = nil

	p.Header.Type = [4]byte{'J', 'P', 'E', 'G'}
	p.Header.Size = int32(len(buf))

	h := bytes.Buffer{}

	binary.Write(&h, binary.LittleEndian, &p.Header)
	binary.Write(&h, binary.LittleEndian, &p.ImageHeader)

	p.Buffer = append(h.Bytes(), buf...)
}

func (p *Packet) FromJPEG(pool *Pool) {
	ctx := C.tj3Init(C.TJINIT_DECOMPRESS)
	defer C.tj3Destroy(ctx)

	C.tj3DecompressHeader(ctx, (*C.uchar)(&p.Buffer[0]), C.size_t(len(p.Buffer)))

	width := int(C.tj3Get(ctx, C.TJPARAM_JPEGWIDTH))
	height := int(C.tj3Get(ctx, C.TJPARAM_JPEGHEIGHT))
	subsampling := C.tj3Get(ctx, C.TJPARAM_SUBSAMP)
	cs := C.tj3Get(ctx, C.TJPARAM_COLORSPACE)

	rectangle := image.Rectangle{
		Max: image.Point{
			X: width,
			Y: height,
		},
	}

	switch cs {
	case C.TJCS_YCbCr:
		s := C.tj3YUVBufSize(C.int(width), 1, C.int(height), subsampling)

		b := pool.Get().(*bytes.Buffer)
		b.Grow(int(s))

		buf := b.Bytes()
		buf = buf[:int(s)]

		switch subsampling {
		case C.TJSAMP_420:
			Y := buf[:width*height]
			Cb := buf[width*height : width*height+width*height/4]
			Cr := buf[width*height+width*height/4:]

			p.Image = &image.YCbCr{
				Rect:           rectangle,
				YStride:        width,
				CStride:        width / 2,
				Y:              Y,
				Cb:             Cb,
				Cr:             Cr,
				SubsampleRatio: image.YCbCrSubsampleRatio420,
			}
		case C.TJSAMP_422:
			Y := buf[:width*height]
			Cb := buf[width*height : width*height+width*height/2]
			Cr := buf[width*height+width*height/2:]

			p.Image = &image.YCbCr{
				Rect:           rectangle,
				YStride:        width,
				CStride:        width / 2,
				Y:              Y,
				Cb:             Cb,
				Cr:             Cr,
				SubsampleRatio: image.YCbCrSubsampleRatio422,
			}
		case C.TJSAMP_444:
			Y := buf[:width*height]
			Cb := buf[width*height : width*height*2]
			Cr := buf[width*height*2:]

			p.Image = &image.YCbCr{
				Rect:           rectangle,
				YStride:        width,
				CStride:        width,
				Y:              Y,
				Cb:             Cb,
				Cr:             Cr,
				SubsampleRatio: image.YCbCrSubsampleRatio422,
			}
		default:
			panic("invalid subsampling")
		}

		C.tj3DecompressToYUV8(ctx, (*C.uchar)(&p.Buffer[0]), C.size_t(len(p.Buffer)), (*C.uchar)(&buf[0]), 1)
	case C.TJCS_RGB:
		s := width * height * 3

		b := pool.Get().(*bytes.Buffer)
		b.Grow(int(s))

		buf := b.Bytes()
		buf = buf[:int(s)]

		p.Image = &image.RGBA{
			Rect:   rectangle,
			Stride: width * 3,
			Pix:    buf,
		}

		C.tj3Decompress8(ctx, (*C.uchar)(&p.Buffer[0]), C.size_t(len(p.Buffer)), (*C.uchar)(&buf[0]), 0, C.TJCS_RGB)
	default:
		panic("invalid colorspace")
	}
}

func (p *Packet) ToImage(w C.uint32_t, h C.uint32_t, format C.enum_video_format, data [C.MAX_AV_PLANES]*C.uint8_t) {
	width := int(w)
	height := int(h)

	rectangle := image.Rectangle{
		Max: image.Point{
			X: width,
			Y: height,
		},
	}

	switch format {
	case C.VIDEO_FORMAT_NV12:
		p.ImageBuffer.Grow(width * height * 3 / 2)

		p.ImageBuffer.Write(unsafe.Slice((*byte)(data[0]), width*height))

		Cb := p.ImageBuffer.Bytes()[width*height : width*height+width*height/4]
		Cr := p.ImageBuffer.Bytes()[width*height+width*height/4 : width*height+width*height/2]

		tmp := unsafe.Slice((*byte)(data[1]), width*height/2)

		for i := 0; i < len(tmp)/2; i++ {
			Cb[i] = tmp[2*i+0]
			Cr[i] = tmp[2*i+1]
		}

		p.Image = &image.YCbCr{
			Rect:           rectangle,
			YStride:        width,
			CStride:        width / 2,
			Y:              p.ImageBuffer.Bytes(),
			Cb:             Cb,
			Cr:             Cr,
			SubsampleRatio: image.YCbCrSubsampleRatio420,
		}
	case C.VIDEO_FORMAT_I420:
		p.ImageBuffer.Write(unsafe.Slice((*byte)(data[0]), width*height))
		p.ImageBuffer.Write(unsafe.Slice((*byte)(data[1]), width*height/4))
		p.ImageBuffer.Write(unsafe.Slice((*byte)(data[2]), width*height/4))

		p.Image = &image.YCbCr{
			Rect:           rectangle,
			YStride:        width,
			CStride:        width / 2,
			Y:              p.ImageBuffer.Bytes()[:width*height],
			Cb:             p.ImageBuffer.Bytes()[width*height : width*height+width*height/4],
			Cr:             p.ImageBuffer.Bytes()[width*height+width*height/4 : width*height+width*height/2],
			SubsampleRatio: image.YCbCrSubsampleRatio420,
		}
	case C.VIDEO_FORMAT_I422:
		p.ImageBuffer.Write(unsafe.Slice((*byte)(data[0]), width*height))
		p.ImageBuffer.Write(unsafe.Slice((*byte)(data[1]), width*height/2))
		p.ImageBuffer.Write(unsafe.Slice((*byte)(data[2]), width*height/2))

		p.Image = &image.YCbCr{
			Rect:           rectangle,
			YStride:        width,
			CStride:        width / 2,
			Y:              p.ImageBuffer.Bytes()[:width*height],
			Cb:             p.ImageBuffer.Bytes()[width*height : width*height+width*height/2],
			Cr:             p.ImageBuffer.Bytes()[width*height+width*height/2:],
			SubsampleRatio: image.YCbCrSubsampleRatio422,
		}
	case C.VIDEO_FORMAT_YVYU:
		p.ImageBuffer.Grow(width * height * 2)

		Y := p.ImageBuffer.Bytes()[:width*height]
		Cb := p.ImageBuffer.Bytes()[width*height : width*height+width*height/2]
		Cr := p.ImageBuffer.Bytes()[width*height+width*height/2 : width*height*2]

		tmp := unsafe.Slice((*byte)(data[0]), width*height*2)

		for i := 0; i < width*height; i++ {
			Y[i] = tmp[i*2]
		}
		for i := 0; i < width*height/2; i++ {
			Cb[i] = tmp[4*i+3]
			Cr[i] = tmp[4*i+1]
		}

		p.Image = &image.YCbCr{
			Rect:           rectangle,
			YStride:        width,
			CStride:        width / 2,
			Y:              Y,
			Cb:             Cb,
			Cr:             Cr,
			SubsampleRatio: image.YCbCrSubsampleRatio422,
		}
	case C.VIDEO_FORMAT_YUY2:
		p.ImageBuffer.Grow(width * height * 2)

		Y := p.ImageBuffer.Bytes()[:width*height]
		Cb := p.ImageBuffer.Bytes()[width*height : width*height+width*height/2]
		Cr := p.ImageBuffer.Bytes()[width*height+width*height/2 : width*height*2]

		tmp := unsafe.Slice((*byte)(data[0]), width*height*2)

		for i := 0; i < width*height; i++ {
			Y[i] = tmp[i*2]
		}
		for i := 0; i < width*height/2; i++ {
			Cb[i] = tmp[4*i+1]
			Cr[i] = tmp[4*i+3]
		}

		p.Image = &image.YCbCr{
			Rect:           rectangle,
			YStride:        width,
			CStride:        width / 2,
			Y:              Y,
			Cb:             Cb,
			Cr:             Cr,
			SubsampleRatio: image.YCbCrSubsampleRatio422,
		}
	case C.VIDEO_FORMAT_UYVY:
		p.ImageBuffer.Grow(width * height * 2)

		Y := p.ImageBuffer.Bytes()[:width*height]
		Cb := p.ImageBuffer.Bytes()[width*height : width*height+width*height/2]
		Cr := p.ImageBuffer.Bytes()[width*height+width*height/2 : width*height*2]

		tmp := unsafe.Slice((*byte)(data[0]), width*height*2)

		for i := 0; i < width*height; i++ {
			Y[i] = tmp[i*2+1]
		}
		for i := 0; i < width*height/2; i++ {
			Cb[i] = tmp[4*i+0]
			Cr[i] = tmp[4*i+2]
		}

		p.Image = &image.YCbCr{
			Rect:           rectangle,
			YStride:        width,
			CStride:        width / 2,
			Y:              Y,
			Cb:             Cb,
			Cr:             Cr,
			SubsampleRatio: image.YCbCrSubsampleRatio422,
		}
	case C.VIDEO_FORMAT_I444:
		p.ImageBuffer.Write(unsafe.Slice((*byte)(data[0]), width*height))
		p.ImageBuffer.Write(unsafe.Slice((*byte)(data[1]), width*height))
		p.ImageBuffer.Write(unsafe.Slice((*byte)(data[2]), width*height))

		p.Image = &image.YCbCr{
			Rect:           rectangle,
			YStride:        width,
			CStride:        width,
			Y:              p.ImageBuffer.Bytes()[:width*height],
			Cb:             p.ImageBuffer.Bytes()[width*height : width*height*2],
			Cr:             p.ImageBuffer.Bytes()[width*height*2 : width*height*3],
			SubsampleRatio: image.YCbCrSubsampleRatio444,
		}
	case C.VIDEO_FORMAT_BGRX:
		p.ImageBuffer.Grow(width * height * 4)

		Pix := p.ImageBuffer.Bytes()[:width*height*4]

		tmp := unsafe.Slice((*byte)(data[0]), width*height*4)

		for i := 0; i < len(tmp); i += 4 {
			Pix[i+0] = tmp[i+2]
			Pix[i+1] = tmp[i+1]
			Pix[i+2] = tmp[i+0]
		}

		p.Image = &image.RGBA{
			Rect:   rectangle,
			Stride: width * 4,
			Pix:    Pix,
		}

	case C.VIDEO_FORMAT_BGRA:
		p.ImageBuffer.Grow(width * height * 4)

		Pix := p.ImageBuffer.Bytes()[:width*height*4]

		tmp := unsafe.Slice((*byte)(data[0]), width*height*4)

		for i := 0; i < len(tmp); i += 4 {
			Pix[i+0] = tmp[i+2]
			Pix[i+1] = tmp[i+1]
			Pix[i+2] = tmp[i+0]
			Pix[i+3] = tmp[i+3]
		}

		p.Image = &image.RGBA{
			Rect:   rectangle,
			Stride: width * 4,
			Pix:    Pix,
		}
	case C.VIDEO_FORMAT_BGR3:
		p.ImageBuffer.Grow(width * height * 4)

		Pix := p.ImageBuffer.Bytes()[:width*height*4]

		tmp := unsafe.Slice((*byte)(data[0]), width*height*4)

		for i := 0; i < len(tmp); i += 4 {
			Pix[i+0] = tmp[i+2]
			Pix[i+1] = tmp[i+1]
			Pix[i+2] = tmp[i+0]
		}

		p.Image = &image.RGBA{
			Rect:   rectangle,
			Stride: width * 4,
			Pix:    Pix,
		}
	case C.VIDEO_FORMAT_RGBA:
		p.ImageBuffer.Write(unsafe.Slice((*byte)(data[0]), width*height*4))

		p.Image = &image.RGBA{
			Rect:   rectangle,
			Stride: width * 4,
			Pix:    p.ImageBuffer.Bytes(),
		}
	default:
		panic("invalid video format")
	}

	return
}

func (p *Packet) ToWAVE(info *C.struct_audio_output_info, frames C.uint32_t, data [C.MAX_AUDIO_CHANNELS]*C.uint8_t) {
	var (
		bytesPerSample int
		format         C.enum_video_format
	)

	switch info.format {
	case C.AUDIO_FORMAT_U8BIT:
		fallthrough
	case C.AUDIO_FORMAT_U8BIT_PLANAR:
		bytesPerSample = 1
		format = C.AUDIO_FORMAT_U8BIT
	case C.AUDIO_FORMAT_16BIT:
		fallthrough
	case C.AUDIO_FORMAT_16BIT_PLANAR:
		bytesPerSample = 2
		format = C.AUDIO_FORMAT_16BIT
	case C.AUDIO_FORMAT_32BIT:
		fallthrough
	case C.AUDIO_FORMAT_32BIT_PLANAR:
		bytesPerSample = 4
		format = C.AUDIO_FORMAT_32BIT
	case C.AUDIO_FORMAT_FLOAT:
		fallthrough
	case C.AUDIO_FORMAT_FLOAT_PLANAR:
		bytesPerSample = 4
		format = C.AUDIO_FORMAT_FLOAT
	}

	p.Header = Header{
		Type:      [4]byte{'W', 'A', 'V', 'E'},
		Timestamp: p.Header.Timestamp,
		Size:      int32(bytesPerSample * int(info.speakers) * int(frames)),
	}

	p.WaveHeader = WaveHeader{
		Format:     int32(format),
		SampleRate: int32(info.samples_per_sec),
		Speakers:   int32(info.speakers),
		Frames:     int32(frames),
	}

	h := bytes.Buffer{}

	binary.Write(&h, binary.LittleEndian, &p.Header)
	binary.Write(&h, binary.LittleEndian, &p.WaveHeader)

	p.Buffer = make([]byte, h.Len()+int(p.Header.Size))

	copy(p.Buffer, h.Bytes())

	wave := p.Buffer[len(p.Buffer)-int(p.Header.Size):]

	switch info.format {
	case C.AUDIO_FORMAT_32BIT_PLANAR:
		fallthrough
	case C.AUDIO_FORMAT_FLOAT_PLANAR:
		var tmp [C.MAX_AUDIO_CHANNELS][]byte

		for i := 0; i < int(info.speakers); i++ {
			tmp[i] = unsafe.Slice((*byte)(data[i]), int(frames)*bytesPerSample)
		}

		for i := 0; i < int(frames); i++ {
			for j := 0; j < int(info.speakers); j++ {
				wave[i*int(info.speakers)*4+j*4+0] = tmp[j][i*4+0]
				wave[i*int(info.speakers)*4+j*4+1] = tmp[j][i*4+1]
				wave[i*int(info.speakers)*4+j*4+2] = tmp[j][i*4+2]
				wave[i*int(info.speakers)*4+j*4+3] = tmp[j][i*4+3]
			}
		}
	case C.AUDIO_FORMAT_16BIT_PLANAR:
		var tmp [C.MAX_AUDIO_CHANNELS][]byte

		for i := 0; i < int(info.speakers); i++ {
			tmp[i] = unsafe.Slice((*byte)(data[i]), int(frames)*bytesPerSample)
		}

		for i := 0; i < int(frames); i++ {
			for j := 0; j < int(info.speakers); j++ {
				wave[i*int(info.speakers)*2+j*2+0] = tmp[j][i*2+0]
				wave[i*int(info.speakers)*2+j*2+1] = tmp[j][i*2+1]
			}
		}
	case C.AUDIO_FORMAT_U8BIT_PLANAR:
		var tmp [C.MAX_AUDIO_CHANNELS][]byte

		for i := 0; i < int(info.speakers); i++ {
			tmp[i] = unsafe.Slice((*byte)(data[i]), int(frames)*bytesPerSample)
		}

		for i := 0; i < int(frames); i++ {
			for j := 0; j < int(info.speakers); j++ {
				wave[i*int(info.speakers)+j] = tmp[j][i]
			}
		}
	default:
		copy(wave, unsafe.Slice((*byte)(data[0]), len(wave)))
	}
}
