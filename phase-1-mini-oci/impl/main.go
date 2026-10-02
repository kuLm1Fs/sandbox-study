package main

import (
	"fmt"
	"os"
)

func usage() {
	fmt.Fprint(os.Stderr, `mini-oci -- 一个最小的 OCI 工具

		用法：
			mini-oci pull <image[:tag]> 拉取镜像，写成 OCI layout
			mini-oci unpack <image[:tag]> 解包 layer，生成 runtime bundle
			mini-oci run <image[:tag]> pull + unpack + runc run


		例：
			mini-oci pull busybox:latest
	`)
}

func main() {
	if len(os.Args) != 3 {
		usage()
		os.Exit(2)
	}

	cmd, ref := os.Args[1], os.Args[2]

	var err error
	switch cmd {
	case "pull":
		err = pull(ref)
	case "unpack":
		err = unpack(ref)
	case "run":
		err = run(ref)
	default:
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "mini-oci: %v\n", err)
		os.Exit(1)
	}
}
