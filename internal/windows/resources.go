//go:build windows

package windows

//go:generate go run github.com/tc-hib/go-winres@v0.3.3 make --in ../../winres/winres.json --arch amd64,arm64 --out rsrc
