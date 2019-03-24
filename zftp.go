//Description: This package is used to transfer multiple text/binary files (using goroutines) in and out of mainframe using FTP.
//Date created: 3/24/2019
//Go Version: 1.12

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"text/template"

	"github.com/gdamore/encoding"
	"github.com/secsy/goftp"
)

//struct used in GET/POST method of main page
type tform3 struct {
	Ftno     int
	Checked1 string
	Checked2 string
	Checked3 string
	Checked4 string
	Dsn      string
	Lrecl    string
	Filname  string
	Size     string
	Message  string
	Failure  bool
}

//struct used in GET method of status page
type tform4 struct {
	Sourcefil string
	Destfil   string
	Success   bool
	Message   string
}

//global scope variables
var machine, userid, pswd string
var failoverall bool
var vform33 = map[int]tform3{}

//function to handle request and responses in main page
func zftp(w http.ResponseWriter, r *http.Request) {

	vform3 := make(map[int]tform3)

	t, err := template.ParseFiles("./views/zftpmain.html")

	if err != nil {
		log.Println(err)
	}

	switch r.Method {
	//process GET method
	case "GET":

		if !failoverall {

			machine = ""
			userid = ""
			pswd = ""
			vform33 = make(map[int]tform3)

			vform3[1] = tform3{
				Ftno:     1,
				Checked1: "checked",
				Checked2: "",
				Checked3: "checked",
				Checked4: "",
				Dsn:      "",
				Lrecl:    "",
				Filname:  "",
				Size:     "",
				Message:  "",
				Failure:  false}

			for key, value := range vform3 {
				vform33[key] = value
			}

		}

		w.Header().Set("Content-Type", "text/html")
		t.Execute(w, map[string]interface{}{"Machine": machine, "Userid": userid, "Pswd": pswd, "Vform33": vform33, "OverallFailure": failoverall})
	//process POST method
	case "POST":

		//initialize
		vform33 = make(map[int]tform3)
		vform3 = make(map[int]tform3)
		failoverall = false
		var message string
		var failure bool
		var wg sync.WaitGroup

		if err := r.ParseForm(); err != nil {
			log.Println(err)
		}

		//get the # of transfers initiated
		var transferNos []int
		for name := range r.PostForm {
			if strings.Contains(name, "trsfrno") {
				atrsfr := strings.Replace(name, "trsfrno", "", -1)
				ntrsfr, _ := strconv.Atoi(atrsfr)
				transferNos = append(transferNos, ntrsfr)
			}
		}

		machine = r.FormValue("machine")
		userid = r.FormValue("userid")
		pswd = r.FormValue("pswd")

		//connect to the FTP server and capture any errors
		config := goftp.Config{
			User:     userid,
			Password: pswd,
		}

		client, err := goftp.DialConfig(config, machine)
		if err != nil {
			message = "Incorrect Hostname/IP address/UserID/password. "
			failure = true
			log.Println(err)
		}

		//for each transfer initiated get the associated form fields into a map
		for _, trsfrno := range transferNos {

			ftpverb := fmt.Sprintf("ftpverb-radio%d", trsfrno)
			ftpformat := fmt.Sprintf("ftpformat-radio%d", trsfrno)
			dsn := fmt.Sprintf("dsn%d", trsfrno)
			lrecl := fmt.Sprintf("lrecl%d", trsfrno)
			filename := fmt.Sprintf("filename%d", trsfrno)

			var checked1, checked2, checked3, checked4 string
			if r.FormValue(ftpverb) == "receive" {
				checked1 = "checked"
				checked2 = ""
			} else if r.FormValue(ftpverb) == "send" {
				checked2 = "checked"
				checked1 = ""
			}

			if r.FormValue(ftpformat) == "text" {
				checked3 = "checked"
				checked4 = ""
			} else if r.FormValue(ftpformat) == "binary" {
				checked4 = "checked"
				checked3 = ""
			}

			vform3[trsfrno] = tform3{
				Ftno:     trsfrno,
				Checked1: checked1,
				Checked2: checked2,
				Checked3: checked3,
				Checked4: checked4,
				Dsn:      r.FormValue(dsn),
				Lrecl:    r.FormValue(lrecl),
				Filname:  r.FormValue(filename),
				Size:     "",
				Message:  message,
				Failure:  failure}

		}

		//initiate transfers using goroutines only if no errors
		if len(message) == 0 {

			for key, val := range vform3 {

				//if transfer is GET text data
				if val.Checked1 == "checked" {
					if val.Checked3 == "checked" {

						wg.Add(1)
						go func(vform3 map[int]tform3, key int, val tform3, failure bool) {
							defer wg.Done()
							log.Println("go routine 1 started...")
							//initialize
							val.Message = ""
							val.Size = ""
							failure = false

							//arrive at dataset name
							dsn := strings.Replace(val.Dsn, "'", "", -1)
							dsn = strings.Replace(dsn, "\"", "", -1)
							dsn = "'" + dsn + "'"

							//create a temporary file
							tmpFile, err := ioutil.TempFile(os.TempDir(), "zftp-")
							if err != nil {
								log.Fatal(err)
							}
							tmpFileName := tmpFile.Name()
							defer os.Remove(tmpFileName)

							//execute GET from mainframe and write to temporary file
							err = client.Retrieve(dsn, tmpFile)
							if err != nil {
								val.Message = val.Message + "Incorrect Source file name. "
								failure = true
								log.Println(err)
							}
							tmpFile.Close()

							//open target file for write
							dest, err := os.OpenFile(val.Filname, os.O_CREATE, 0666)
							if err != nil {
								val.Message = val.Message + "Incorrect Destination file name. "
								failure = true
								log.Println(err)
							}

							if len(val.Message) == 0 {

								//open temporary file for read
								tmpFile, err = os.OpenFile(tmpFileName, os.O_RDONLY, 0666)
								if err != nil {
									log.Println(err)
								}

								//create another reader from io.reader
								tmpFileRdr := bufio.NewReader(tmpFile)

								//slice the bytes to be read based on LRECL
								lrecl, err := strconv.Atoi(val.Lrecl)
								if err != nil {
									log.Println(err)
								}
								buf := make([]byte, 0, lrecl)

								//read LRECL bytes from temporary file until EOF
								for {
									n, err := io.ReadFull(tmpFileRdr, buf[:cap(buf)])
									buf = buf[:n]
									if err != nil {
										if err == io.EOF {
											break
										}
										if err != io.ErrUnexpectedEOF {
											log.Println(err)
											break
										}
									}

									//decode EBCDIC to UTF-8
									decode, err := encoding.EBCDIC.NewDecoder().Bytes(buf)
									if err != nil {
										log.Println(err)
									}

									//convert bytes to string and write it to target file
									str := string(decode[:])
									if runtime.GOOS == "windows" {
										_, err = dest.WriteString(str + "\r\n")
										if err != nil {
											log.Println(err)
										}
									} else {
										_, err = dest.WriteString(str + "\n")
										if err != nil {
											log.Println(err)
										}
									}

								}

								dest.Close()

								dest, err = os.OpenFile(val.Filname, os.O_RDONLY, 0666)
								if err != nil {
									log.Println(err)
								}
								stat, err := dest.Stat()
								if err != nil {
									log.Println(err)
								}

								var size float64
								size = float64(stat.Size()) / 1024.00

								val.Size = fmt.Sprintf("%.2f", size)

							}

							vform3[key] = tform3{Checked1: val.Checked1,
								Checked2: val.Checked2,
								Checked3: val.Checked3,
								Checked4: val.Checked4,
								Dsn:      val.Dsn,
								Lrecl:    val.Lrecl,
								Filname:  val.Filname,
								Size:     val.Size,
								Message:  val.Message,
								Failure:  failure}

							log.Println("go routine 1 completed...")
						}(vform3, key, val, failure)

					}
				}

				//if transfer is PUT text
				if val.Checked2 == "checked" {
					if val.Checked3 == "checked" {

						wg.Add(1)
						go func(vform3 map[int]tform3, key int, val tform3, failure bool) {
							defer wg.Done()
							log.Println("go routine 2 started...")

							//initialize
							val.Message = ""
							val.Size = ""
							failure = false

							//arrive at dataset name
							dsn := strings.Replace(val.Dsn, "'", "", -1)
							dsn = strings.Replace(dsn, "\"", "", -1)
							dsn = "'" + dsn + "'"

							//open the source file
							srcfil, err := os.Open(val.Filname)
							if err != nil {
								val.Message = val.Message + "Incorrect Source file name. "
								failure = true
								log.Println(err)
							}

							//create a temporary file
							tmpFile, err := ioutil.TempFile(os.TempDir(), "zftp-")
							if err != nil {
								log.Fatal(err)
							}
							tmpFileName := tmpFile.Name()
							defer os.Remove(tmpFileName)

							//read the source file one line at a time and encode it to EBCDIC
							scanner := bufio.NewScanner(srcfil)
							lrecl, err := strconv.Atoi(val.Lrecl)
							if err != nil {
								log.Println(err)
							}

							for scanner.Scan() {
								inline := scanner.Text()
								inline = strings.Replace(inline, "\r", "", -1)
								inline = strings.Replace(inline, "\n", "", -1)
								inline = strings.TrimSpace(inline)

								var rembytes string
								if len(inline) >= lrecl {
									inline = inline[:lrecl]
								} else {
									popspace := lrecl - len(inline)
									spaces := strings.Repeat(" ", popspace)
									rembytes = spaces
								}

								line := inline + rembytes

								encode, err := encoding.EBCDIC.NewEncoder().String(line)
								if err != nil {
									log.Println(err)
								}
								tmpFile.WriteString(encode)
							}

							tmpFile.Close()

							//open temporary file for read
							tmpFile2, err := ioutil.ReadFile(tmpFileName)
							if err != nil {
								log.Println(err)
							}

							if len(val.Message) == 0 {
								//execute PUT to mainframe
								err = client.Store(dsn, bytes.NewReader(tmpFile2))
								if err != nil {
									val.Message = val.Message + "Incorrect Source file name. "
									failure = true
									log.Println(err)
								}

								stat, err := srcfil.Stat()
								if err != nil {
									log.Println(err)
								}

								var size float64
								size = float64(stat.Size()) / 1024.00
								val.Size = fmt.Sprintf("%.2f", size)

							}

							vform3[key] = tform3{Checked1: val.Checked1,
								Checked2: val.Checked2,
								Checked3: val.Checked3,
								Checked4: val.Checked4,
								Dsn:      val.Dsn,
								Lrecl:    val.Lrecl,
								Filname:  val.Filname,
								Size:     val.Size,
								Message:  val.Message,
								Failure:  failure}

							log.Println("go routine 2 completed...")

						}(vform3, key, val, failure)

					}

				}

				//if transfer is GET binary data
				if val.Checked1 == "checked" {
					if val.Checked4 == "checked" {

						wg.Add(1)
						go func(vform3 map[int]tform3, key int, val tform3, failure bool) {
							defer wg.Done()
							log.Println("go routine 3 started...")

							//initialize
							val.Message = ""
							val.Size = ""
							failure = false

							//arrive at dataset name
							dsn := strings.Replace(val.Dsn, "'", "", -1)
							dsn = strings.Replace(dsn, "\"", "", -1)
							dsn = "'" + dsn + "'"

							//open target file for write
							dest, err := os.OpenFile(val.Filname, os.O_CREATE, 0666)
							if err != nil {
								val.Message = val.Message + "Incorrect Destination file name. "
								failure = true
								log.Println(err)
							}

							if len(val.Message) == 0 {
								//execute GET from mainframe and write to target file
								err = client.Retrieve(dsn, dest)
								if err != nil {
									val.Message = val.Message + "Incorrect Source file name. "
									failure = true
									log.Println(err)
								}

								dest.Close()

								dest, err = os.OpenFile(val.Filname, os.O_RDONLY, 0666)
								if err != nil {
									log.Println(err)
								}
								stat, err := dest.Stat()
								if err != nil {
									log.Println(err)
								}

								var size float64
								size = float64(stat.Size()) / 1024.00

								val.Size = fmt.Sprintf("%.2f", size)

							}

							vform3[key] = tform3{Checked1: val.Checked1,
								Checked2: val.Checked2,
								Checked3: val.Checked3,
								Checked4: val.Checked4,
								Dsn:      val.Dsn,
								Lrecl:    val.Lrecl,
								Filname:  val.Filname,
								Size:     val.Size,
								Message:  val.Message,
								Failure:  failure}

							log.Println("go routine 3 completed...")

						}(vform3, key, val, failure)

					}

				}

				//if transfer is PUT binary data
				if val.Checked2 == "checked" {
					if val.Checked4 == "checked" {

						wg.Add(1)
						go func(vform3 map[int]tform3, key int, val tform3, failure bool) {
							defer wg.Done()
							log.Println("go routine 4 started...")

							//initialize
							val.Message = ""
							val.Size = ""
							failure = false

							//arrive at dataset name
							dsn := strings.Replace(val.Dsn, "'", "", -1)
							dsn = strings.Replace(dsn, "\"", "", -1)
							dsn = "'" + dsn + "'"

							//open source file for read
							source, err := os.OpenFile(val.Filname, os.O_RDONLY, 0666)
							if err != nil {
								val.Message = val.Message + "Incorrect Source file name. "
								failure = true
								log.Println(err)
							}

							//execute PUT to mainframe
							if len(val.Message) == 0 {
								err = client.Store(dsn, source)
								if err != nil {
									val.Message = val.Message + "Incorrect Destination file name. "
									failure = true
									log.Println(err)
								}

								stat, err := source.Stat()
								if err != nil {
									log.Println(err)
								}

								var size float64
								size = float64(stat.Size()) / 1024.00
								val.Size = fmt.Sprintf("%.2f", size)

							}

							vform3[key] = tform3{Checked1: val.Checked1,
								Checked2: val.Checked2,
								Checked3: val.Checked3,
								Checked4: val.Checked4,
								Dsn:      val.Dsn,
								Lrecl:    val.Lrecl,
								Filname:  val.Filname,
								Size:     val.Size,
								Message:  val.Message,
								Failure:  failure}

							log.Println("go routine 4 completed...")

						}(vform3, key, val, failure)

					}

				}

			}

		}

		//wait for all go routines to complete
		log.Println("waiting for goroutines to complete...")
		wg.Wait()
		log.Println("all goroutines completed...")

		for key, value := range vform3 {
			vform33[key] = value
		}

		http.Redirect(w, r, "/zftp/status", http.StatusSeeOther)
	}

}

//function to handle request and responses in status page
func zftpstat(w http.ResponseWriter, r *http.Request) {

	t, err := template.ParseFiles("./views/zftpstatus.html")

	if err != nil {
		log.Println(err)
	}

	Stats := make(map[int]tform4)

	var sourcefil, destfil, message string
	var success bool

	for key, val := range vform33 {

		if val.Checked1 == "checked" {
			sourcefil = val.Dsn
			destfil = val.Filname

		} else if val.Checked2 == "checked" {
			sourcefil = val.Filname
			destfil = val.Dsn
		}

		if len(val.Message) > 0 {
			success = false
			message = val.Message
			failoverall = true
		} else {
			success = true
			message = val.Size + " KB transferred."
		}

		Stats[key] = tform4{
			Sourcefil: sourcefil,
			Destfil:   destfil,
			Success:   success,
			Message:   message}

	}

	switch r.Method {
	//process GET method
	case "GET":

		w.Header().Set("Content-Type", "text/html")
		t.Execute(w, map[string]interface{}{"Stats": Stats, "OverallFailure": failoverall})

	}
}

func main() {

	fs := http.FileServer(http.Dir("static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))
	http.Handle("/zftp/static/", http.StripPrefix("/zftp/static/", fs))
	http.HandleFunc("/zftp", zftp)
	http.HandleFunc("/zftp/status", zftpstat)

	log.Println("listening on 9001...")
	http.ListenAndServe(":9001", nil)

	return
}
