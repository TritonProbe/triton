package packet

func DecodePacketNumber(largestPN, truncatedPN uint64, pnBits int) uint64 {
	expected := largestPN + 1
	pnWin := uint64(1) << pnBits
	pnHWin := pnWin / 2
	pnMask := pnWin - 1
	candidate := (expected & ^pnMask) | truncatedPN

	if candidate+pnHWin <= expected && candidate < (1<<62)-pnWin {
		return candidate + pnWin
	}
	if candidate > expected+pnHWin && candidate >= pnWin {
		return candidate - pnWin
	}
	return candidate
}

func EncodePacketNumber(fullPN, largestAcked uint64) (uint64, int) {
	unacked := fullPN - largestAcked
	switch {
	case unacked < 0x80:
		return fullPN & 0xff, 8
	case unacked < 0x8000:
		return fullPN & 0xffff, 16
	case unacked < 0x800000:
		return fullPN & 0xffffff, 24
	default:
		return fullPN & 0xffffffff, 32
	}
}
