package goad

import "github.com/giorgisio/goav/avformat"

func Init() {
	avformat.AvRegisterAll()
}
