package libraries

import "os"

// Windows file identity is compared with os.SameFile within media groups.
func healthLinkCount(_ os.FileInfo) uint64 { return 0 }
