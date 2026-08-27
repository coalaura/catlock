//go:build windows

package main

//go:generate go run github.com/tc-hib/go-winres@v0.3.3 simply --arch amd64,arm64 --out rsrc --manifest gui --product-name CatLock --file-description "A keyboard lock for cats" --icon assets/catlock.png
