package main

//
// #include <obs-module.h>
//
import "C"
import (
	"bytes"
	"encoding/binary"
	"image"
	"unsafe"

	"github.com/pixiv/go-libjpeg/jpeg"
	"github.com/pixiv/go-libjpeg/rgb"
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

func (p *Packet) ToJPEG() {
	b := bytes.Buffer{}

	jpeg.Encode(&b, p.Image, &jpeg.EncoderOptions{
		Quality: p.Quality,
	})

	p.Image = nil

	p.Header.Type = [4]byte{'J', 'P', 'E', 'G'}
	p.Header.Size = int32(b.Len())

	h := bytes.Buffer{}

	binary.Write(&h, binary.LittleEndian, &p.Header)
	binary.Write(&h, binary.LittleEndian, &p.ImageHeader)

	p.Buffer = append(h.Bytes(), b.Bytes()...)
}

func (p *Packet) FromJPEG() {
	r := bytes.NewReader(p.Buffer)
	p.Image, _ = jpeg.Decode(r, &jpeg.DecoderOptions{})
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
			Pix[i+3] = 0xff
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
		p.ImageBuffer.Grow(width * height * 3)

		Pix := p.ImageBuffer.Bytes()[:width*height*3]

		tmp := unsafe.Slice((*byte)(data[0]), width*height*3)

		for i := 0; i < len(tmp); i += 3 {
			Pix[i+0] = tmp[i+2]
			Pix[i+1] = tmp[i+1]
			Pix[i+2] = tmp[i+0]
		}

		p.Image = &rgb.Image{
			Rect:   rectangle,
			Stride: width * 3,
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
		panic("")
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
