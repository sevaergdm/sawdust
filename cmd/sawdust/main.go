package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "footer":
		cmdFooter(os.Args[2:])
	case "schema":
		cmdSchema(os.Args[2:])
	case "stat":
		cmdStat(os.Args[2:])
	case "survey":
		cmdSurvey(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "error: unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}

}

func usage() {
	fmt.Fprint(os.Stderr, `usage: sawdust <command> [arguments]

Commands:
  footer <file>   print the file's byte layout
  schema <file>   print the schema tree
	stat 	 <file> 	print file stats
	survey <dir>		print stats for all parquet files in directory tree
`)
}
