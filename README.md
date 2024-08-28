# Fileserver

Asymmetric fileserver golang package

## Description

This is a package for golang.
Asymmetric fileserver allows upload of files without classic authentication.
It is intended for use where reciever is expecting sender to upload some files. Files can only be downloaded by receiver.
Reciever generates a unique code and initializes upload with the server (PUT /init/). Here an authentication is needed.
Then, the reciever communicates the code to the sender, which can use it as impromptu authentication to upload a file in one piece or multiple parts.

### Installation

Once you have [installed Go][golang-install], run this command
to install the `fileserver` package:

    go get github.com/Hrnkas/fileserver
    
### Documentation

## Implementig your own server using fileserver package

```golang

func checkAuthentication(w http.ResponseWriter, req *http.Request) bool {
	username, password, ok := req.BasicAuth()
	if !ok {
		w.WriteHeader(http.StatusForbidden)
		return false
	}
	if username != os.Getenv("FILEUP_USER") {
		w.WriteHeader(http.StatusForbidden)
		return false
	}
	password_hash, err := hex.DecodeString(os.Getenv("FILEUP_PASSWORD_SHA512"))
	password_hash_check := sha512.Sum512([]byte(password))
	if err != nil || !bytes.Equal(password_hash_check[:], password_hash) {
		w.WriteHeader(http.StatusForbidden)
		return false
	}

	return true
}

var db *gorm.DB

func main() {
	var err error

	//create directories
	newpath := filepath.Dir("/data/db/")
	err = os.MkdirAll(newpath, os.ModePerm)
	if err != nil {
		panic(err)
	}

	newpath = filepath.Dir("/data/uploads/")
	err = os.MkdirAll(newpath, os.ModePerm)
	if err != nil {
		panic(err)
	}

	db, err = gorm.Open(sqlite.Open("/data/db/uploads.db"), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	fs, err := fileserver.NewFileserver("/data/uploads/", db, checkAuthentication)
	if err != nil {
		panic(err)
	}

	log.Fatal(fs.Serve())
}
```

If you want to set up your own routes, you can set up your own mux and just use exported handler functions.

```golang
	fs, err := fileserver.NewFileserver("/data/uploads/", db, checkAuthentication)
	if err != nil {
		panic(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /info/{code}", fs.GetFileInfo)
	mux.HandleFunc("GET /download/{code}/{part}", fs.DownloadPart)
	mux.HandleFunc("GET /download/{code}", fs.DownloadFile)
	mux.HandleFunc("PUT /upload/{code}/{part}", fs.Store)
	mux.HandleFunc("PUT /init/", fs.InitUpload)
	mux.HandleFunc("DELETE /delete/{code}", fs.DeleteUpload)

	log.Fatal(http.ListenAndServe(":8080", mux))
```

## Server Usage

### Initialise Upload
PUT /init/

Payload is JSON:
`{ "Code": "myuniquecode", "Filename": "somefilename" }`

Requires Authentication

### Provide other party with your unique code

### Other party uploads files
PUT /upload/{code}/{part}

Part name can be freely chosen, but should be different for all parts of the same code.

Expected payload is application/octet-stream.

### Get the list of uploaded parts
GET /info/{code}

Requires Authentication

### Download parts
GET /download/{code}/{part}

Requires Authentication


-- or -- 

### Download all parts as a single file
GET /download/{code}

Requires Authentication

### Delete all file parts and database records of the given code
DELETE /delete/{code}

Requires Authentication




**Author:** Bojan Hrnkas – [Hrnkas](https://github.com/Hrnkas)

[golang-install]: http://golang.org/doc/install.html
