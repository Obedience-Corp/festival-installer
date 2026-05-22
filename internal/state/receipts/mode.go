package receipts

import "os"

func osFileMode(v int64) os.FileMode {
	return os.FileMode(uint32(v))
}
