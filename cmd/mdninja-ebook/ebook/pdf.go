package ebook

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	pdfcpuapi "github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	pdfcpumodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	pdfcputypes "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"markdown.ninja/stdx/filex"
)

func ebookToPdf(ctx context.Context, config Config, pandocFiles pandocFiles, distPath, coverPath string) (err error) {
	if !strings.HasSuffix(distPath, ".pdf") {
		err = fmt.Errorf("error generating pdf: destination file (%s) must end with .pdf", distPath)
		return
	}

	pdfFilenameWithExtension := strings.TrimSuffix(filepath.Base(distPath), ".pdf")

	tmpContentPdfPath, err := tmpFile(config.tmpWorkingDir, pdfFilenameWithExtension+"-*")
	if err != nil {
		return
	}
	tmpContentPdfPath += "_content.pdf"

	tmpCoverPdfPath, err := tmpFile(config.tmpWorkingDir, pdfFilenameWithExtension+"-*")
	if err != nil {
		return
	}
	tmpCoverPdfPath += "_cover.pdf"

	err = ConvertCoverToPdf(ctx, tmpCoverPdfPath, coverPath)
	if err != nil {
		return
	}

	args := []string{pandocFiles.settingsPath}
	args = append(args, config.Chapters...)
	args = append(args, "--output="+tmpContentPdfPath)
	// see https://pandoc.org/MANUAL.html#option--pdf-engine for other options
	args = append(args, "--pdf-engine=xelatex")
	args = append(args, "--strip-comments")
	args = append(args, "--table-of-contents")
	args = append(args, "--toc-depth=2")
	args = append(args, "--number-sections")
	args = append(args, "--top-level-division=chapter")
	args = append(args, "--include-in-header="+pandocFiles.inlineCodeTexPath)
	args = append(args, "-V", "fontsize=12pt")
	// the documentclass=book produces weird results and too many empty pages...
	// args = append(args, "-V", "documentclass=book")
	args = append(args, "-V", "documentclass=report")
	args = append(args, "-V", "linkcolor:blue")
	args = append(args, "--highlight-style="+pandocFiles.themePath)
	args = append(args, "-M", "date="+config.Version)
	// if we use the --file-scope flag, footnotes/references can't be consigned in a different file
	// (references.md for example). On the other hand, it forces footnotes to be globally uniques as they
	// will all share the same scope
	// args = append(args, "--file-scope")
	// args = append(args, "-V", "links-as-notes=true")
	// args = append(args, "--reference-links")            // not working...
	// args = append(args, "--reference-location=section") // not working... only for epub
	// args = append(args, "--highlight-style=tango")

	cmd := exec.Command("pandoc", args...)
	cmd.Env = ebooksSandboxEnv()
	cmd.Dir = config.tmpWorkingDir
	cmdOutput, err := cmd.CombinedOutput()
	if err != nil {
		output := string(cmdOutput)
		err = fmt.Errorf("store.ebookToPdf: creating pdf: %w -> %s", err, output)
		return
	}

	err = MergePdfs(ctx, distPath, tmpCoverPdfPath, tmpContentPdfPath)
	if err != nil {
		return
	}

	// os.Remove(coverPdfPath)
	// os.Remove(contentPdfPath)

	return
}

func ConvertCoverToPdf(ctx context.Context, destPath, sourcePath string) (err error) {
	sourceExists, err := filex.Exists(sourcePath)
	if err != nil {
		err = fmt.Errorf("checking if %s exists: %w", sourcePath, err)
		return
	}
	if !sourceExists {
		err = fmt.Errorf("%s: file does not exist", sourcePath)
		return
	}

	if !strings.HasSuffix(destPath, ".pdf") {
		err = fmt.Errorf("destination file (%s) must end with .pdf", destPath)
		return
	}

	// cmd := exec.Command("img2pdf", source, "--imgsize", "216mmx297mm", "-o", dest) // a4 size slightly modified for pandoc
	// cmdOutput, err := cmd.CombinedOutput()
	// if err != nil {
	// 	output = string(cmdOutput)
	// 	err = fmt.Errorf("md2pdf: converting cover to pdf: %w", err)
	// 	return
	// }
	// pdfcpu import cover.pdf cover.png
	err = pdfcpuapi.ImportImagesFile(ctx, []string{sourcePath}, destPath, nil, nil)
	if err != nil {
		err = fmt.Errorf("converting cover to PDF: %w", err)
		return
	}

	// pdfcpu resize -u mm -- 'dimensions:216 279, enforce:true' cover.pdf cover.pdf
	// resizeDim := pdfcputypes.Dim{
	// 	Width:  pdfcputypes.ToUserSpace(216.0, pdfcputypes.MILLIMETRES)
	// 	Height: pdfcputypes.ToUserSpace(279.0, pdfcputypes.MILLIMETRES)
	// }
	// resizeOpts := pdfcpumodel.Resize{
	// 	Unit:          pdfcputypes.MILLIMETRES,
	// 	PageDim:       &resizeDim,
	// 	UserDim:       true,
	// 	EnforceOrient: true,
	// }
	// // conf := pdfcpumodel.NewDefaultConfiguration()
	// // conf.Unit = pdfcputypes.MILLIMETRES
	resizeOpts, err := pdfcpu.ParseResizeConfig("dimensions:216 279, enforce:true", pdfcputypes.MILLIMETRES)
	if err != nil {
		err = fmt.Errorf("error parsing cover resize config: %w", err)
		return
	}

	err = pdfcpuapi.ResizeFile(ctx, destPath, destPath, nil, resizeOpts, nil)
	if err != nil {
		err = fmt.Errorf("error resizing cover: %w", err)
		return
	}

	return nil
}

func MergePdfs(ctx context.Context, destPath, coverPath, contentPath string) (err error) {
	coverExists, err := filex.Exists(coverPath)
	if err != nil {
		err = fmt.Errorf("checking if %s exists: %w", coverPath, err)
		return
	}
	if !coverExists {
		err = fmt.Errorf("%s: file does not exist", coverPath)
		return
	}
	if !strings.HasSuffix(coverPath, ".pdf") {
		err = fmt.Errorf("cover file (%s) must end with .pdf", coverPath)
		return
	}

	contentExist, err := filex.Exists(contentPath)
	if err != nil {
		err = fmt.Errorf("checking if %s exists: %w", contentPath, err)
		return
	}
	if !contentExist {
		err = fmt.Errorf("%s: file does not exist", contentPath)
		return
	}
	if !strings.HasSuffix(contentPath, ".pdf") {
		err = fmt.Errorf("content file (%s) must end with .pdf", contentPath)
		return
	}

	if !strings.HasSuffix(destPath, ".pdf") {
		err = fmt.Errorf("destination file (%s) must end with .pdf", destPath)
		return
	}

	// 	cmd = exec.Command("pdftk", coverPdfPath, pdfContentPath, "cat", "output", m.config.distFilePdf)
	// cmdOutput, err = cmd.CombinedOutput()
	// if err != nil {
	// 	output = string(cmdOutput)
	// 	err = fmt.Errorf("md2pdf: merging cover and book's content: %w", err)
	// 	return
	// }

	mergeConf := pdfcpumodel.NewDefaultConfiguration()
	err = pdfcpuapi.MergeCreateFile(ctx, []string{coverPath, contentPath}, destPath, false, mergeConf)
	if err != nil {
		err = fmt.Errorf("error merging cover and content: %w", err)
		return
	}
	return nil
}
