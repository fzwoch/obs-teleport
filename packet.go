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
