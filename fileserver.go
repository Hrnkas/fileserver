package fileserver

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Upload struct {
	gorm.Model `json:"-"`
	Code       string `gorm:"uniqueIndex"`
	Filename   string
}

type Part struct {
	gorm.Model `json:"-"`
	PartCode   string `gorm:"uniqueIndex:idx_part"`
	UploadID   uint   `gorm:"uniqueIndex:idx_part"`
	Upload     Upload
}

type CheckAuth func(w http.ResponseWriter, req *http.Request) bool

type Fileserver struct {
	db         *gorm.DB
	checkAuth  CheckAuth
	uploadpath string
}

// NewService - our constructor function
func NewFileserver(uploadpath string, db *gorm.DB, checkAuth CheckAuth) (*Fileserver, error) {
	server := &Fileserver{
		db:         db,
		checkAuth:  checkAuth,
		uploadpath: uploadpath,
	}

	err := server.db.AutoMigrate(&Upload{}, &Part{}) //create tables

	return server, err
}

func (fs Fileserver) getRegisteredUpload(code_upload string) (Upload, error) {
	var up Upload
	err := fs.db.First(&up, "code = ?", code_upload).Error
	return up, err
}

func (fs Fileserver) getAllRegisteredUploads() ([]Upload, error) {
	var uploads []Upload
	err := fs.db.Find(&uploads).Error
	return uploads, err
}

func (fs Fileserver) getUploadParts(upload Upload) ([]Part, error) {
	var parts []Part
	err := fs.db.Where("upload_id = ?", upload.ID).Order("part_code").Find(&parts).Error
	return parts, err
}

func (fs Fileserver) getUploadPart(upload Upload, code_part string) (Part, error) {
	var part Part
	err := fs.db.Where("upload_id = ?", upload.ID).Where("part_code = ?", code_part).First(&part).Error
	return part, err
}

func (fs Fileserver) Store(w http.ResponseWriter, req *http.Request) {

	code_upload := req.PathValue("code")
	code_part := req.PathValue("part")
	if strings.TrimSpace(code_upload) == "" || strings.TrimSpace(code_part) == "" {
		//error invalid request
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Parameters must not be empty."))
		return
	}

	code_upload = sanitizeFilename(code_upload)
	code_part = sanitizeFilename(code_part)

	upload, err := fs.getRegisteredUpload(code_upload)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Upload with given code could not be found."))
		return
	}

	filepath := fs.getPartFilename(upload.Code, code_part)
	f, err := os.OpenFile(filepath, os.O_RDWR|os.O_CREATE, 0644)
	//lint:ignore SA5001 - I dont care about error from file close
	defer f.Close()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("File " + filepath + " could not be written. Check the server configuration."))
		return
	}
	_, err = io.Copy(f, req.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("File " + filepath + " could not be written. Check the server configuration."))
		return
	}

	err = fs.db.Create(&Part{PartCode: code_part, UploadID: upload.ID}).Error
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Database write error"))

		//cleanup (delete file)
		os.Remove(filepath) //error can be ignored at this stage

		return
	}
}

func (fs Fileserver) InitUpload(w http.ResponseWriter, req *http.Request) {

	if !fs.checkAuth(w, req) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	var up Upload
	err := json.NewDecoder(req.Body).Decode(&up)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	//sanitize input
	up.Code = sanitizeFilename(up.Code)
	up.Filename = sanitizeFilename(up.Filename)

	errCreate := fs.db.Create(&up).Error
	if errCreate != nil {
		http.Error(w, errCreate.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	errEncode := json.NewEncoder(w).Encode(up)
	if errEncode != nil {
		http.Error(w, errEncode.Error(), http.StatusInternalServerError)
		return
	}
}

func (fs Fileserver) GetFileInfo(w http.ResponseWriter, req *http.Request) {
	if !fs.checkAuth(w, req) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	code_upload := req.PathValue("code")
	if strings.TrimSpace(code_upload) == "" {
		//error invalid request
		http.Error(w, "Parameters must not be empty.", http.StatusBadRequest)
		return
	}

	code_upload = sanitizeFilename(code_upload)

	upload, err := fs.getRegisteredUpload(code_upload)
	if err != nil {
		http.Error(w, "Upload with given code could not be found.", http.StatusNotFound)
		return
	}

	//get upload info, including part list, and return as json
	parts, errParts := fs.getUploadParts(upload)

	if errParts != nil && errParts != gorm.ErrRecordNotFound {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	type UploadWithDate struct {
		Upload
		LastUpload time.Time
	}

	type FileInfoResult struct {
		Upload UploadWithDate
		Parts  []Part
	}

	//find the latest uploaded part
	// Assume the first part has the latest CreatedAt initially
	latestPart := parts[len(parts)-1]

	uploadWithDate := UploadWithDate{Upload: upload, LastUpload: latestPart.CreatedAt}
	result := FileInfoResult{Upload: uploadWithDate, Parts: parts}

	w.Header().Set("Content-Type", "application/json")
	errEncode := json.NewEncoder(w).Encode(result)
	if errEncode != nil {
		http.Error(w, errEncode.Error(), http.StatusInternalServerError)
		return
	}
}

func (fs Fileserver) GetFileInfoList(w http.ResponseWriter, req *http.Request) {
	if !fs.checkAuth(w, req) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	uploads, err := fs.getAllRegisteredUploads()
	if err != nil {
		http.Error(w, "Upload list could not be retrieved.", http.StatusNotFound)
		return
	}

	type FileInfoListResult struct {
		Uploads []Upload
	}

	result := FileInfoListResult{Uploads: uploads}

	w.Header().Set("Content-Type", "application/json")
	errEncode := json.NewEncoder(w).Encode(result)
	if errEncode != nil {
		http.Error(w, errEncode.Error(), http.StatusInternalServerError)
		return
	}
}

func (fs Fileserver) DownloadPart(w http.ResponseWriter, req *http.Request) {
	if !fs.checkAuth(w, req) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	code_upload := req.PathValue("code")
	code_part := req.PathValue("part")
	if strings.TrimSpace(code_upload) == "" || strings.TrimSpace(code_part) == "" {
		//error invalid request
		http.Error(w, "Parameters must not be empty.", http.StatusBadRequest)
		return
	}

	upload, err := fs.getRegisteredUpload(code_upload)
	if err != nil {
		http.Error(w, "Upload with given code could not be found.", http.StatusNotFound)
		return
	}

	part, err := fs.getUploadPart(upload, code_part)
	if err != nil {
		http.Error(w, "Upload part with given code could not be found.", http.StatusNotFound)
		return
	}

	filename_instore := fs.getPartFilename(upload.Code, part.PartCode)

	f, err := os.Open(filename_instore)
	if err != nil {
		http.Error(w, "Can not open file", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	filename_download := upload.Filename + "." + part.PartCode

	//todo-maybe: make GetFileSize function
	stat, err := f.Stat()
	if err != nil {
		http.Error(w, "Can not read file size", http.StatusInternalServerError)
		return
	}
	size := stat.Size()

	w.Header().Set("Content-Disposition", "attachment; filename="+filename_download)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))

	_, errCopy := io.Copy(w, f) //write file to response writer
	if errCopy != nil {
		http.Error(w, "Can not read file", http.StatusInternalServerError)
		return
	}
}

func (fs Fileserver) DownloadFile(w http.ResponseWriter, req *http.Request) {

	if !fs.checkAuth(w, req) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	code_upload := req.PathValue("code")
	if strings.TrimSpace(code_upload) == "" {
		//error invalid request
		http.Error(w, "Parameters must not be empty.", http.StatusBadRequest)
		return
	}

	upload, err := fs.getRegisteredUpload(code_upload)
	if err != nil {
		http.Error(w, "Upload with given code could not be found.", http.StatusNotFound)
		return
	}

	parts, err := fs.getUploadParts(upload)
	if err != nil {
		http.Error(w, "Upload part with given code could not be found.", http.StatusNotFound)
		return
	}

	//get total file length
	var length int64
	for _, part := range parts {
		filename_instore := fs.getPartFilename(upload.Code, part.PartCode)

		stat, err := os.Stat(filename_instore)
		if err != nil {
			http.Error(w, "Can not read file size", http.StatusInternalServerError)
			return
		}

		length += stat.Size()
	}

	filename_download := upload.Filename

	w.Header().Set("Content-Disposition", "attachment; filename="+filename_download)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(length, 10))

	for _, part := range parts {
		filename_instore := fs.getPartFilename(upload.Code, part.PartCode)

		f, err := os.Open(filename_instore)
		if err != nil {
			http.Error(w, "Can not open file", http.StatusInternalServerError)
		}
		defer f.Close()

		_, errCopy := io.Copy(w, f) //write file to response writer
		if errCopy != nil {
			http.Error(w, "Can not read file", http.StatusInternalServerError)
		}
	}
}

func (fs Fileserver) DeleteUpload(w http.ResponseWriter, req *http.Request) {

	if !fs.checkAuth(w, req) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	code_upload := req.PathValue("code")
	if strings.TrimSpace(code_upload) == "" {
		//error invalid request
		http.Error(w, "Parameters must not be empty.", http.StatusBadRequest)
		return
	}

	upload, err := fs.getRegisteredUpload(code_upload)
	if err != nil {
		http.Error(w, "Upload with given code could not be found.", http.StatusNotFound)
		return
	}

	//remove from database
	parts, errDB := fs.getUploadParts(upload)
	if errDB != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
	}

	for _, part := range parts {
		//remove file on disk
		filename_instore := fs.getPartFilename(upload.Code, part.PartCode)
		_ = os.Remove(filename_instore) //safe to ignore error

		fs.db.Delete(&part)
		fs.db.Unscoped().Delete(&part)
	}

	fs.db.Delete(&upload)
	fs.db.Unscoped().Delete(&upload)

}

func (fs Fileserver) Serve() error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /info/", fs.GetFileInfoList)
	mux.HandleFunc("GET /info/{code}", fs.GetFileInfo)
	mux.HandleFunc("GET /download/{code}/{part}", fs.DownloadPart)
	mux.HandleFunc("GET /download/{code}", fs.DownloadFile)
	mux.HandleFunc("PUT /upload/{code}/{part}", fs.Store)
	mux.HandleFunc("PUT /init/", fs.InitUpload)
	mux.HandleFunc("DELETE /delete/{code}", fs.DeleteUpload)

	return http.ListenAndServe(":8080", mux)
}

func (fs Fileserver) getPartFilename(name, ext string) string {
	return fs.uploadpath + "/" + name + "." + ext
}

func sanitizeFilename(filename string) string {
	pathNameRegExp := regexp.MustCompile(`[^a-zA-Z0-9-_\.]`)
	return string(pathNameRegExp.ReplaceAll([]byte(filename), []byte("")))
}
