package utils

//BitsBuffer bits buffer
type BitsBuffer struct {
	iSize int
	iData int
	iMask uint8
	pData []byte
}

func BitsInit(isize int, buffer []byte) *BitsBuffer {

	bits := &BitsBuffer{
		iSize: isize,
		iData: 0,
		iMask: 0x80,
		pData: buffer,
	}
	if bits.pData == nil {
		bits.pData = make([]byte, isize)
	}
	return bits
}

func BitsWrite(bits *BitsBuffer, count int, src uint64) *BitsBuffer {

	for count > 0 {
		count--
		if ((src >> uint(count)) & 0x01) != 0 {
			bits.pData[bits.iData] |= bits.iMask
		} else {
			bits.pData[bits.iData] &= ^bits.iMask
		}
		bits.iMask >>= 1
		if bits.iMask == 0 {
			bits.iData++
			bits.iMask = 0x80
		}
	}

	return bits
}
