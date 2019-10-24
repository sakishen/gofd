package common

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"path"
)

// TarGz and tar from source directory or file to destination file
// you need check file exist before you call this function
func TarGz(srcDirPath string, destFilePath string) {
	fw, err := os.Create(destFilePath)
	handleError(err)
	defer fw.Close()

	// Gzip writer
	gw := gzip.NewWriter(fw)
	defer gw.Close()

	// Tar writer
	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Check if it's a file or a directory
	f, err := os.Open(srcDirPath)
	handleError(err)
	fi, err := f.Stat()
	handleError(err)
	if fi.IsDir() {
		// handle source directory
		fmt.Println("Creating tar.gz from directory...")
		tarGzDir(srcDirPath, path.Base(srcDirPath), tw)
	} else {
		// handle file directly
		fmt.Println("Creating tar.gz from " + fi.Name() + "...")
		tarGzFile(srcDirPath, fi.Name(), tw, fi)
	}
	fmt.Println("TarGz done!")
}

// Deal with directories
// if find files, handle them with tarGzFile
// Every recurrence append the base path to the recPath
// recPath is the path inside of tar.gz
func tarGzDir(srcDirPath string, recPath string, tw *tar.Writer) {
	// Open source directory
	dir, err := os.Open(srcDirPath)
	handleError(err)
	defer dir.Close()

	// Get file info slice
	fis, err := dir.Readdir(0)
	handleError(err)
	for _, fi := range fis {
		// Append path
		curPath := srcDirPath + "/" + fi.Name()
		// Check it is directory or file
		if fi.IsDir() {
			// Directory (Directory won't add until all sub files are added)
			fmt.Printf("Adding path...%s\n", curPath)
			tarGzDir(curPath, recPath+"/"+fi.Name(), tw)
		} else {
			// File
			fmt.Printf("Adding file...%s\n", curPath)
		}

		tarGzFile(curPath, recPath+"/"+fi.Name(), tw, fi)
	}
}

// Deal with files
func tarGzFile(srcFile string, recPath string, tw *tar.Writer, fi os.FileInfo) {
	if fi.IsDir() {
		// Create tar header
		hdr := new(tar.Header)
		// if last character of header name is '/' it also can be directory
		// but if you don't set Type_flag, error will occur when you decompression
		hdr.Name = recPath + "/"
		hdr.Typeflag = tar.TypeDir
		hdr.Size = 0
		//hdr.Mode = 0755 | c_ISDIR
		hdr.Mode = int64(fi.Mode())
		hdr.ModTime = fi.ModTime()
		//Uid   int    // User ID of owner
		//Gid   int    // Group ID of owner

		// Write handler
		err := tw.WriteHeader(hdr)
		handleError(err)
	} else {
		// File reader
		fr, err := os.Open(srcFile)
		handleError(err)
		defer fr.Close()

		// Create tar header
		hdr := new(tar.Header)
		hdr.Name = recPath
		hdr.Size = fi.Size()
		hdr.Mode = int64(fi.Mode())
		hdr.ModTime = fi.ModTime()

		// Write handler
		err = tw.WriteHeader(hdr)
		handleError(err)

		// Write file data
		_, err = io.Copy(tw, fr)
		handleError(err)
	}
}

// UnTarGz and untar from source file to destination directory
// you need check file exist before you call this function
func UnTarGz(srcFilePath string, destDirPath string, uid int, gid int) error {
	fmt.Println("UnTarGzing " + srcFilePath + "...")
	// Create destination directory
	if _, err := os.Stat(destDirPath); !os.IsNotExist(err) {
		_ = os.Mkdir(destDirPath, os.ModePerm)
	} else {
		fmt.Println("dir path already exists.")
	}

	fr, err := os.Open(srcFilePath)
	handleError(err)
	defer fr.Close()

	// Gzip reader
	gr, err := gzip.NewReader(fr)
	handleError(err)
	defer gr.Close()

	// Tar reader
	tr := tar.NewReader(gr)

	for hdr, er := tr.Next(); er != io.EOF; hdr, er = tr.Next() {
		if er != nil {
			handleError(er)
		}

		// 获取文件信息
		fi := hdr.FileInfo()

		// 获取绝对路径
		dstFullPath := destDirPath + "/" + hdr.Name

		// 判断是否为文件夹或文件
		if hdr.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(dstFullPath, fi.Mode().Perm()); err != nil {
				handleError(err)
			}
			// 设置目录权限
			if err := os.Chmod(dstFullPath, fi.Mode().Perm()); err != nil {
				handleError(err)
			}
			// 设置目录权限
			if err := os.Chown(dstFullPath, uid, gid); err != nil {
				handleError(err)
			}
		} else {
			// 创建文件所在的目录
			_ = os.MkdirAll(path.Dir(dstFullPath), os.ModePerm)
			// 将 tr 中的数据写入文件中
			if er := unTarFile(dstFullPath, tr); er != nil {
				return er
			}
			// 设置文件权限
			if err := os.Chmod(dstFullPath, fi.Mode().Perm()); err != nil {
				handleError(err)
			}
			// 设置目录权限
			if err := os.Chown(dstFullPath, uid, gid); err != nil {
				handleError(err)
			}
		}
	}
	log.Println("UnTarGz done!")
	return nil
}

// 因为要在 defer 中关闭文件，所以要单独创建一个函数
func unTarFile(dstFile string, tr *tar.Reader) error {
	// 创建空文件，准备写入解包后的数据
	fw, er := os.Create(dstFile)
	if er != nil {
		return er
	}
	defer fw.Close()

	// 写入解包后的数据, 当目标文件内容和原文件不符之时 目标文件会被原文件覆盖。
	_, er = io.Copy(fw, tr)
	if er != nil {
		return er
	}

	return nil
}

func handleError(err error) {
	if err != nil {
		log.Println("err:", err)
	}
}
