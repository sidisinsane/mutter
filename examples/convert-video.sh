#!/usr/bin/env bash
# ---
# description: Convert video files to different formats using ffmpeg
# usage: convert-video.sh --input {{input}} --output {{output}}
# type:
#   input: [video]
#   output: [video]
# arguments:
#   input:
#     pattern: '(?i)(?:convert|input)\s+(\S+\.(mp4|avi|mov|mkv))'
#     description: Input video file path
#   output:
#     pattern: '(?i)(?:to|output)\s+(\S+\.(mp4|avi|mov|mkv))'
#     description: Output video file path
# exits:
#   0: success
#   1: ffmpeg not found
#   2: input file not found
#   3: conversion failed
# ---

INPUT=""
OUTPUT=""

while [[ $# -gt 0 ]]; do
  case $1 in
    --input)
      INPUT="$2"
      shift 2
      ;;
    --output)
      OUTPUT="$2"
      shift 2
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

if [ -z "$INPUT" ] || [ -z "$OUTPUT" ]; then
  echo "Usage: convert-video.sh --input <input> --output <output>"
  exit 1
fi

if [ ! -f "$INPUT" ]; then
  echo "Error: Input file not found: $INPUT"
  exit 2
fi

if ! command -v ffmpeg &> /dev/null; then
  echo "Error: ffmpeg not found. Please install ffmpeg."
  exit 1
fi

ffmpeg -i "$INPUT" -c:v copy -c:a copy "$OUTPUT" -y
exit $?
