#!/bin/sh


if [ -z "$1" ]; then
  echo "Must provide a target path."
  echo "Usage: $0 <path>"
  exit 1
fi

go run ./cmd/genfix -out "$1" -file_name basic       -num_rows 100
go run ./cmd/genfix -out "$1" -file_name empty       -num_rows 0
go run ./cmd/genfix -out "$1" -file_name single_row  -num_rows 1
go run ./cmd/genfix -out "$1" -file_name many_rows   -num_rows 300 -row_group_size=100
go run ./cmd/genfix -out "$1" -file_name nested      -num_rows 100 -kind=nested
go run ./cmd/genfix -out "$1" -file_name zstd        -num_rows 100 -compression=zstd
go run ./cmd/genfix -out "$1" -file_name multi_page  -num_rows 100 -page_buffer_size=256
