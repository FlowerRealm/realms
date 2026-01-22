package assets

import _ "embed"

//go:embed realms_icon.svg
var realmsIconSVG []byte

func RealmsIconSVG() []byte {
	return realmsIconSVG
}
