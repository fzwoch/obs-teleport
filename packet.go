package main

//
// #include <obs-module.h>
//
import "C"
import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
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

	paddedHeight := height + 16

	rectangle := image.Rectangle{
		Max: image.Point{
			X: width,
			Y: height,
		},
	}

	switch format {
	case C.VIDEO_FORMAT_NV12:
		p.Image = &image.YCbCr{
			Rect:           rectangle,
			YStride:        width,
			CStride:        width / 2,
			Y:              make([]byte, width*paddedHeight),
			Cb:             make([]byte, width*paddedHeight/4),
			Cr:             make([]byte, width*paddedHeight/4),
			SubsampleRatio: image.YCbCrSubsampleRatio420,
		}

		copy(p.Image.(*image.YCbCr).Y, unsafe.Slice((*byte)(data[0]), width*height))

		tmp := unsafe.Slice((*byte)(data[1]), width*height/2)

		for i := 0; i < len(tmp)/2; i++ {
			p.Image.(*image.YCbCr).Cb[i] = tmp[2*i+0]
			p.Image.(*image.YCbCr).Cr[i] = tmp[2*i+1]
		}
	case C.VIDEO_FORMAT_I420:
		p.Image = &image.YCbCr{
			Rect:           rectangle,
			YStride:        width,
			CStride:        width / 2,
			Y:              make([]byte, width*paddedHeight),
			Cb:             make([]byte, width*paddedHeight/4),
			Cr:             make([]byte, width*paddedHeight/4),
			SubsampleRatio: image.YCbCrSubsampleRatio420,
		}

		copy(p.Image.(*image.YCbCr).Y, unsafe.Slice((*byte)(data[0]), width*height))
		copy(p.Image.(*image.YCbCr).Cb, unsafe.Slice((*byte)(data[1]), width*height/4))
		copy(p.Image.(*image.YCbCr).Cr, unsafe.Slice((*byte)(data[2]), width*height/4))
	case C.VIDEO_FORMAT_I422:
		p.Image = &image.YCbCr{
			Rect:           rectangle,
			YStride:        width,
			CStride:        width / 2,
			Y:              make([]byte, width*paddedHeight),
			Cb:             make([]byte, width*paddedHeight/2),
			Cr:             make([]byte, width*paddedHeight/2),
			SubsampleRatio: image.YCbCrSubsampleRatio422,
		}

		copy(p.Image.(*image.YCbCr).Y, unsafe.Slice((*byte)(data[0]), width*height))
		copy(p.Image.(*image.YCbCr).Cb, unsafe.Slice((*byte)(data[1]), width*height/2))
		copy(p.Image.(*image.YCbCr).Cr, unsafe.Slice((*byte)(data[2]), width*height/2))
	case C.VIDEO_FORMAT_YVYU:
		p.Image = &image.YCbCr{
			Rect:           rectangle,
			YStride:        width,
			CStride:        width / 2,
			Y:              make([]byte, width*paddedHeight),
			Cb:             make([]byte, width*paddedHeight/2),
			Cr:             make([]byte, width*paddedHeight/2),
			SubsampleRatio: image.YCbCrSubsampleRatio422,
		}

		tmp := unsafe.Slice((*byte)(data[0]), width*height*2)

		for i := 0; i < width*height; i++ {
			p.Image.(*image.YCbCr).Y[i] = tmp[i*2]
		}
		for i := 0; i < width*height/2; i++ {
			p.Image.(*image.YCbCr).Cb[i] = tmp[4*i+3]
			p.Image.(*image.YCbCr).Cr[i] = tmp[4*i+1]
		}
	case C.VIDEO_FORMAT_YUY2:
		p.Image = &image.YCbCr{
			Rect:           rectangle,
			YStride:        width,
			CStride:        width / 2,
			Y:              make([]byte, width*paddedHeight),
			Cb:             make([]byte, width*paddedHeight/2),
			Cr:             make([]byte, width*paddedHeight/2),
			SubsampleRatio: image.YCbCrSubsampleRatio422,
		}

		tmp := unsafe.Slice((*byte)(data[0]), width*height*2)

		for i := 0; i < width*height; i++ {
			p.Image.(*image.YCbCr).Y[i] = tmp[i*2]
		}
		for i := 0; i < width*height/2; i++ {
			p.Image.(*image.YCbCr).Cb[i] = tmp[4*i+1]
			p.Image.(*image.YCbCr).Cr[i] = tmp[4*i+3]
		}
	case C.VIDEO_FORMAT_UYVY:
		p.Image = &image.YCbCr{
			Rect:           rectangle,
			YStride:        width,
			CStride:        width / 2,
			Y:              make([]byte, width*paddedHeight),
			Cb:             make([]byte, width*paddedHeight/2),
			Cr:             make([]byte, width*paddedHeight/2),
			SubsampleRatio: image.YCbCrSubsampleRatio422,
		}

		tmp := unsafe.Slice((*byte)(data[0]), width*height*2)

		for i := 0; i < width*height; i++ {
			p.Image.(*image.YCbCr).Y[i] = tmp[i*2+1]
		}
		for i := 0; i < width*height/2; i++ {
			p.Image.(*image.YCbCr).Cb[i] = tmp[4*i+0]
			p.Image.(*image.YCbCr).Cr[i] = tmp[4*i+2]
		}
	case C.VIDEO_FORMAT_I444:
		p.Image = &image.YCbCr{
			Rect:           rectangle,
			YStride:        width,
			CStride:        width,
			Y:              make([]byte, width*paddedHeight),
			Cb:             make([]byte, width*paddedHeight),
			Cr:             make([]byte, width*paddedHeight),
			SubsampleRatio: image.YCbCrSubsampleRatio444,
		}

		copy(p.Image.(*image.YCbCr).Y, unsafe.Slice((*byte)(data[0]), width*height))
		copy(p.Image.(*image.YCbCr).Cb, unsafe.Slice((*byte)(data[1]), width*height))
		copy(p.Image.(*image.YCbCr).Cr, unsafe.Slice((*byte)(data[2]), width*height))
	case C.VIDEO_FORMAT_BGRX:
		p.Image = &image.RGBA{
			Rect:   rectangle,
			Stride: width * 4,
			Pix:    make([]byte, width*paddedHeight*4),
		}

		tmp := unsafe.Slice((*byte)(data[0]), width*height*4)

		for i := 0; i < len(tmp); i += 4 {
			p.Image.(*image.RGBA).Pix[i+0] = tmp[i+2]
			p.Image.(*image.RGBA).Pix[i+1] = tmp[i+1]
			p.Image.(*image.RGBA).Pix[i+2] = tmp[i+0]
			p.Image.(*image.RGBA).Pix[i+3] = 0xff
		}
	case C.VIDEO_FORMAT_BGRA:
		p.Image = &image.RGBA{
			Rect:   rectangle,
			Stride: width * 4,
			Pix:    make([]byte, width*paddedHeight*4),
		}

		tmp := unsafe.Slice((*byte)(data[0]), width*height*4)

		for i := 0; i < len(tmp); i += 4 {
			p.Image.(*image.RGBA).Pix[i+0] = tmp[i+2]
			p.Image.(*image.RGBA).Pix[i+1] = tmp[i+1]
			p.Image.(*image.RGBA).Pix[i+2] = tmp[i+0]
			p.Image.(*image.RGBA).Pix[i+3] = tmp[i+3]
		}
	case C.VIDEO_FORMAT_BGR3:
		p.Image = &rgb.Image{
			Rect:   rectangle,
			Stride: width * 3,
			Pix:    make([]byte, width*paddedHeight*3),
		}

		tmp := unsafe.Slice((*byte)(data[0]), width*height*3)

		for i := 0; i < len(tmp); i += 3 {
			p.Image.(*rgb.Image).Pix[i+0] = tmp[i+2]
			p.Image.(*rgb.Image).Pix[i+1] = tmp[i+1]
			p.Image.(*rgb.Image).Pix[i+2] = tmp[i+0]
		}
	case C.VIDEO_FORMAT_RGBA:
		p.Image = &image.RGBA{
			Rect:   rectangle,
			Stride: width * 4,
			Pix:    make([]byte, width*paddedHeight*4),
		}

		copy(p.Image.(*image.RGBA).Pix, unsafe.Slice((*byte)(data[0]), width*height*4))
	default:
		p.Image = image.NewRGBA(
			rectangle,
		)
		for x := 0; x < width; x++ {
			for y := 0; y < height; y++ {
				color := color.RGBA{
					uint8(255 * x / width),
					uint8(255 * y / height),
					55,
					255}
				p.Image.(*image.RGBA).Set(x, y, color)
			}
		}
	}

	return
}

func (p *Packet) ToWAVE(info *C.struct_audio_output_info, frames C.uint32_t, data [C.MAX_AV_PLANES]*C.uint8_t) {
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
