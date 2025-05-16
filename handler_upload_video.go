package main

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {

	reader := http.MaxBytesReader(w, r.Body, 1<<30)

	defer reader.Close()

	videoIdStr := r.PathValue("videoID")

	videoId, err := uuid.Parse(videoIdStr)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Coundn't parse UUID from video ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)

	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid bearer token", err)
		return
	}

	userId, err := auth.ValidateJWT(token, cfg.jwtSecret)

	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate token", err)
		return
	}

	video, err := cfg.db.GetVideo(videoId)

	if err != nil {
		respondWithError(w, http.StatusNotFound, "Couldn't find video with ID", err)
		return
	}

	if video.UserID != userId {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	videoFile, header, err := r.FormFile("video")

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error reading formfile", err)
		return
	}

	defer videoFile.Close()

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't parse the header for `Content-Type`", err)
		return
	}

	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusUnsupportedMediaType, "Type is not mp4", nil)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload*.mp4")

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create temp file", err)
		return
	}

	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	_, err = io.Copy(tempFile, videoFile)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't copy file", err)
		return
	}

	_, err = tempFile.Seek(0, io.SeekStart)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error reseting file read offset", err)
		return
	}

	processedVideoPath, err := processVideoForFastStart(tempFile.Name())

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error processing file", err)
		return
	}

	processedVideoFile, err := os.Open(processedVideoPath)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error opening processed file", err)
		return
	}

	defer os.Remove(processedVideoPath)
	defer processedVideoFile.Close()

	base := make([]byte, 32)
	_, err = rand.Read(base)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error creating random number", err)
		return
	}

	aspectRatio, err := getVideoAspectRatio(tempFile.Name())

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "No able to get video aspect ratio", err)
		return
	}

	var orientation string

	switch aspectRatio {
	case "16:9":
		orientation = "landscape/"
	case "9:16":
		orientation = "portrait/"
	default:
		orientation = "portrait/"
	}

	splittedContentType := strings.Split(mediaType, "/")
	dstFileExt := splittedContentType[1]
	dstFileName := orientation + base64.RawURLEncoding.EncodeToString(base) + "." + dstFileExt

	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      aws.String(cfg.s3Bucket),
		Key:         aws.String(dstFileName),
		Body:        processedVideoFile,
		ContentType: aws.String(mediaType),
	})

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't upload to CDN", err)
		return
	}

	videoURL := cfg.s3CfDistribution + dstFileName
	video.VideoURL = &videoURL

	err = cfg.db.UpdateVideo(video)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video on db", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
