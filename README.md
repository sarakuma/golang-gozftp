# gozftp - A FTP client for mainframe
repo hold scripts that are needed to transfer multiple text/binary files (concurrently) in and out of mainframe using FTP

## Go version
1.12.1

## Dependencies
* github.com/gdamore/encoding
* github.com/secsy/goftp

## Usage
with mainframe ftp-server

## Benefits
* implemented go-routines for quick turn-around
* simultaneous gets and puts
* suffice error handling and feedback

## Instructions
1. at terminal - go build zftp.go 
2. run zftp.exe
3. launch browser @socket - http://localhost:9001/zftp

### To-dos
* improve error handling
* improve html layout & aesthetics
* increase the max limit to 50
* include append functionality




