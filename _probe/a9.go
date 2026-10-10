package main

import "os"

func writeA9(a9 []byte, path string) { os.WriteFile(path, a9, 0o644) }
