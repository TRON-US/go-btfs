package corehttp

import (
	"context"
	"fmt"
	"github.com/TRON-US/go-btfs/namesys/resolve"
	"io"
	"net/http"
	"net/url"
	"os"
	gopath "path"
	"runtime/debug"
	"strings"
	"time"

	files "github.com/TRON-US/go-btfs-files"
	"github.com/TRON-US/go-mfs"
	coreiface "github.com/TRON-US/interface-go-btfs-core"
	ipath "github.com/TRON-US/interface-go-btfs-core/path"
	"github.com/Workiva/go-datastructures/cache"
	"github.com/dustin/go-humanize"
	"github.com/ipfs/go-cid"
	dag "github.com/ipfs/go-merkledag"
	ipfspath "github.com/ipfs/go-path"
	"github.com/ipfs/go-path/resolver"
	routing "github.com/libp2p/go-libp2p-core/routing"
	"github.com/multiformats/go-multibase"
)

const (
	ipfsPathPrefix                           = "/btfs/"
	ipnsPathPrefix                           = "/btns/"
	GatewayReedSolomonDirectoryCacheCapacity = 10
)

// gatewayHandler is a HTTP handler that serves BTFS objects (accessible by default at /btfs/<path>)
// (it serves requests like GET /btfs/QmVRzPKPzNtSrEzBFm2UZfxmPAgnaLke4DMcerbsGGSaFe/link)
type gatewayHandler struct {
	config GatewayConfig
	api    coreiface.CoreAPI
	rsDirs cache.Cache
}

type ReedSolomonDirectory struct {
	rootPath ipath.Resolved
	rootDir  files.Directory
}

func (d ReedSolomonDirectory) Size() uint64 {
	// Returns one since this Size() is used by the cache.Cache
	// to indicate one directory entry in the cache.
	return uint64(1)
}

func newGatewayHandler(c GatewayConfig, api coreiface.CoreAPI, dirs cache.Cache) *gatewayHandler {
	i := &gatewayHandler{
		config: c,
		api:    api,
		rsDirs: dirs,
	}
	return i
}

func parseIpfsPath(p string) (cid.Cid, string, error) {
	rootPath, err := ipfspath.ParsePath(p)
	if err != nil {
		return cid.Cid{}, "", err
	}

	// Check the path.
	rsegs := rootPath.Segments()
	if rsegs[0] != "btfs" {
		return cid.Cid{}, "", fmt.Errorf("WritableGateway: only btfs paths supported")
	}

	rootCid, err := cid.Decode(rsegs[1])
	if err != nil {
		return cid.Cid{}, "", err
	}

	return rootCid, ipfspath.Join(rsegs[2:]), nil
}

func (i *gatewayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// the hour is a hard fallback, we don't expect it to happen, but just in case
	ctx, cancel := context.WithTimeout(r.Context(), time.Hour)
	defer cancel()
	r = r.WithContext(ctx)

	defer func() {
		if r := recover(); r != nil {
			log.Error("A panic occurred in the gateway handler!")
			log.Error(r)
			debug.PrintStack()
		}
	}()

	if i.config.Writable {
		switch r.Method {
		case "POST":
			i.postHandler(w, r)
			return
		case "PUT":
			i.putHandler(w, r)
			return
		case "DELETE":
			i.deleteHandler(w, r)
			return
		}
	}

	if r.Method == "GET" || r.Method == "HEAD" {
		i.getOrHeadHandler(w, r)
		return
	}

	if r.Method == "OPTIONS" {
		i.optionsHandler(w, r)
		return
	}

	errmsg := "Method " + r.Method + " not allowed: "
	var status int
	if !i.config.Writable {
		status = http.StatusMethodNotAllowed
		errmsg = errmsg + "read only access"
	} else {
		status = http.StatusBadRequest
		errmsg = errmsg + "bad request for " + r.URL.Path
	}
	http.Error(w, errmsg, status)
}

func (i *gatewayHandler) optionsHandler(w http.ResponseWriter, r *http.Request) {
	/*
		OPTIONS is a noop request that is used by the browsers to check
		if server accepts cross-site XMLHttpRequest (indicated by the presence of CORS headers)
		https://developer.mozilla.org/en-US/docs/Web/HTTP/Access_control_CORS#Preflighted_requests
	*/
	i.addUserHeaders(w) // return all custom headers (including CORS ones, if set)
}

func (i *gatewayHandler) getOrHeadHandler(w http.ResponseWriter, r *http.Request) {
	begin := time.Now()
	urlPath := r.URL.Path
	escapedURLPath := r.URL.EscapedPath()

	// If the gateway is behind a reverse proxy and mounted at a sub-path,
	// the prefix header can be set to signal this sub-path.
	// It will be prepended to links in directory listings and the index.html redirect.
	prefix := ""
	if prfx := r.Header.Get("X-Ipfs-Gateway-Prefix"); len(prfx) > 0 {
		for _, p := range i.config.PathPrefixes {
			if prfx == p || strings.HasPrefix(prfx, p+"/") {
				prefix = prfx
				break
			}
		}
	}

	// IPNSHostnameOption might have constructed an BTNS path using the Host header.
	// In this case, we need the original path for constructing redirects
	// and links that match the requested URL.
	// For example, http://example.net would become /ipns/example.net, and
	// the redirects and links would end up as http://example.net/ipns/example.net
	originalUrlPath := prefix + urlPath
	ipnsHostname := false
	if hdr := r.Header.Get("X-Ipns-Original-Path"); len(hdr) > 0 {
		originalUrlPath = prefix + hdr
		ipnsHostname = true
	}

	parsedPath := ipath.New(urlPath)
	if err := parsedPath.IsValid(); err != nil {
		webError(w, "invalid btfs path", err, http.StatusBadRequest)
		return
	}

	// Resolve path to the final DAG node for the ETag
	var (
		resolvedPath              ipath.Resolved
		rootPath                  string
		escapedRootPath           string
		resolvedRootPath          ipath.Resolved
		dr                        files.Node
		err                       error
		isReedSolomonSubdirOrFile bool // Is ReedSolomon non-root subdirectory or file?
	)
	top, err := i.isTopLevelEntryPath(r, parsedPath.String())
	if err != nil {
		switch err {
		case nil:
		case coreiface.ErrOffline:
			webError(w, "btfs resolve -r "+escapedURLPath, err, http.StatusServiceUnavailable)
			return
		default:
			webError(w, "btfs resolve -r "+escapedURLPath, err, http.StatusNotFound)
			return
		}
	}
	if !top {
		// Check if the `parsedPath` is part of Reed-Solomon enconded directory object.
		// If yes, put a new entry to the Reed-Solomon directory cache if not exist and
		// get the files.Node of the `parsedPath`.
		k := strings.SplitN(parsedPath.String(), "/", 4)[2]
		v, isPresent, err := i.cacheEntryFor(k)
		if err != nil {
			internalWebError(w, err)
			return
		}
		var rootDir files.Directory
		escapedRootPath, err = getDirRootPath(escapedURLPath)
		if err != nil {
			webError(w, "invalid btfs path without slashes", err, http.StatusBadRequest)
			return
		}
		if isPresent {
			resolvedRootPath, rootDir = v.rootPath, v.rootDir
		} else {
			rootPath, err = getDirRootPath(parsedPath.String())
			if err != nil {
				webError(w, "invalid btfs path without slashes", err, http.StatusBadRequest)
				return
			}

			var rootDr files.Node
			resolvedRootPath, rootDr, err = i.resolveAndGetFilesNode(context.Background(), w, r, ipath.New(rootPath), escapedRootPath)
			if err != nil {
				return
			}
			ok := false
			rootDir, ok = rootDr.(files.Directory)
			if !ok {
				log.Warningf("expected directory type for %s", escapedRootPath)
			}
		}
		if isPresent || (rootDir != nil && rootDir.IsReedSolomon()) {
			if !isPresent {
				// Put the current Reed-Solomon directory entry to the cache
				i.rsDirs.Put(k, ReedSolomonDirectory{rootPath: resolvedRootPath, rootDir: rootDir})
			}
			// Get the files.Node of the `parsedPath`.
			dr, err = findNode(rootDir, parsedPath.String())
			if err != nil {
				internalWebError(w, err)
				return
			}
			isReedSolomonSubdirOrFile = true
		}
	}

	if !isReedSolomonSubdirOrFile {
		resolvedPath, dr, err = i.resolveAndGetFilesNode(r.Context(), w, r, parsedPath, escapedURLPath)
		if err != nil {
			return
		}
	}

	unixfsGetMetric.WithLabelValues(parsedPath.Namespace()).Observe(time.Since(begin).Seconds())

	defer dr.Close()

	// Check etag send back to us.
	if isReedSolomonSubdirOrFile {
		// Set root path prefix for paths for the entity tag.
		i.setupEntityTag(w, r, resolvedRootPath, rootPath)
	} else {
		i.setupEntityTag(w, r, resolvedPath, urlPath)
	}

	// Suborigin header, sandboxes apps from each other in the browser (even
	// though they are served from the same gateway domain).
	//
	// Omitted if the path was treated by IPNSHostnameOption(), for example
	// a request for http://example.net/ would be changed to /ipns/example.net/,
	// which would turn into an incorrect Suborigin header.
	// In this case the correct thing to do is omit the header because it is already
	// handled correctly without a Suborigin.
	//
	// NOTE: This is not yet widely supported by browsers.
	if !ipnsHostname {
		// e.g.: 1="btfs", 2="QmYuNaKwY...", ...
		pathComponents := strings.SplitN(urlPath, "/", 4)

		var suboriginRaw []byte
		cidDecoded, err := cid.Decode(pathComponents[2])
		if err != nil {
			// component 2 doesn't decode with cid, so it must be a hostname
			suboriginRaw = []byte(strings.ToLower(pathComponents[2]))
		} else {
			suboriginRaw = cidDecoded.Bytes()
		}

		base32Encoded, err := multibase.Encode(multibase.Base32, suboriginRaw)
		if err != nil {
			internalWebError(w, err)
			return
		}

		suborigin := pathComponents[1] + "000" + strings.ToLower(base32Encoded)
		w.Header().Set("Suborigin", suborigin)
	}

	// set these headers _after_ the error, for we may just not have it
	// and dont want the client to cache a 500 response...
	// and only if it's /btfs!
	// TODO: break this out when we split /btfs /btns routes.
	modtime := time.Now()

	if f, ok := dr.(files.File); ok {
		if strings.HasPrefix(urlPath, ipfsPathPrefix) {
			w.Header().Set("Cache-Control", "public, max-age=29030400, immutable")

			// set modtime to a really long time ago, since files are immutable and should stay cached
			modtime = time.Unix(1, 0)
		}

		urlFilename := r.URL.Query().Get("filename")
		var name string
		if urlFilename != "" {
			w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename*=UTF-8''%s", url.PathEscape(urlFilename)))
			name = urlFilename
		} else {
			name = getFilename(urlPath)
		}
		i.serveFile(w, r, name, modtime, f)
		return
	}
	dir, ok := dr.(files.Directory)
	if !ok {
		internalWebError(w, fmt.Errorf("unsupported file type"))
		return
	}

	if !isReedSolomonSubdirOrFile {
		idx, err := i.api.Unixfs().Get(r.Context(), ipath.Join(resolvedPath, "index.html"))
		switch err.(type) {
		case nil:
			dirwithoutslash := urlPath[len(urlPath)-1] != '/'
			goget := r.URL.Query().Get("go-get") == "1"
			if dirwithoutslash && !goget {
				// See comment above where originalUrlPath is declared.
				http.Redirect(w, r, originalUrlPath+"/", 302)
				return
			}

			f, ok := idx.(files.File)
			if !ok {
				internalWebError(w, files.ErrNotReader)
				return
			}

			// write to request
			http.ServeContent(w, r, "index.html", modtime, f)
			return
		case resolver.ErrNoLink:
			// no index.html; noop
		default:
			internalWebError(w, err)
			return
		}
	}

	if r.Method == "HEAD" {
		return
	}

	// storage for directory list
	var dirListing []directoryItem
	dirit := dir.Entries()
	dirit.BreadthFirstTraversal()
	for dirit.Next() {
		// See comment above where originalUrlPath is declared.
		s, err := dirit.Node().Size()
		if err != nil {
			internalWebError(w, err)
			return
		}

		di := directoryItem{humanize.Bytes(uint64(s)), dirit.Name(), gopath.Join(originalUrlPath, dirit.Name())}
		dirListing = append(dirListing, di)
	}
	if dirit.Err() != nil {
		internalWebError(w, dirit.Err())
		return
	}

	// Put the current Reed-Solomon directory entry to the cache
	// if it is Reed-Solomon root directory.
	if !isReedSolomonSubdirOrFile && dir.IsReedSolomon() {
		dr, err := i.api.Unixfs().Get(context.Background(), resolvedPath)
		if err != nil {
			webError(w, "btfs cat "+escapedURLPath, err, http.StatusNotFound)
			return
		}
		ok := false
		dir, ok = dr.(files.Directory)
		if !ok {
			internalWebError(w, fmt.Errorf("expected directory type for %s", escapedURLPath))
			return
		}

		i.rsDirs.Put(strings.SplitN(parsedPath.String(), "/", 4)[2],
			ReedSolomonDirectory{rootPath: resolvedPath, rootDir: dir})
	}

	// construct the correct back link
	// https://github.com/ipfs/go-ipfs/issues/1365
	var backLink string = prefix + urlPath

	// don't go further up than /btfs/$hash/
	pathSplit := ipfspath.SplitList(backLink)
	switch {
	// keep backlink
	case len(pathSplit) == 3: // url: /btfs/$hash

	// keep backlink
	case len(pathSplit) == 4 && pathSplit[3] == "": // url: /btfs/$hash/

	// add the correct link depending on wether the path ends with a slash
	default:
		if strings.HasSuffix(backLink, "/") {
			backLink += "./.."
		} else {
			backLink += "/.."
		}
	}

	// strip /btfs/$hash from backlink if IPNSHostnameOption touched the path.
	if ipnsHostname {
		backLink = prefix + "/"
		if len(pathSplit) > 5 {
			// also strip the trailing segment, because it's a backlink
			backLinkParts := pathSplit[3 : len(pathSplit)-2]
			backLink += ipfspath.Join(backLinkParts) + "/"
		}
	}

	var hash string
	if !strings.HasPrefix(originalUrlPath, ipfsPathPrefix) {
		hash = resolvedPath.Cid().String()
	}

	// See comment above where originalUrlPath is declared.
	tplData := listingTemplateData{
		Listing:  dirListing,
		Path:     originalUrlPath,
		BackLink: backLink,
		Hash:     hash,
	}
	err = listingTemplate.Execute(w, tplData)
	if err != nil {
		internalWebError(w, err)
		return
	}
}

type sizeReadSeeker interface {
	Size() (int64, error)

	io.ReadSeeker
}

type sizeSeeker struct {
	sizeReadSeeker
}

func (s *sizeSeeker) Seek(offset int64, whence int) (int64, error) {
	if whence == io.SeekEnd && offset == 0 {
		return s.Size()
	}

	return s.sizeReadSeeker.Seek(offset, whence)
}

func (i *gatewayHandler) serveFile(w http.ResponseWriter, req *http.Request, name string, modtime time.Time, content io.ReadSeeker) {
	if sp, ok := content.(sizeReadSeeker); ok {
		content = &sizeSeeker{
			sizeReadSeeker: sp,
		}
	}

	http.ServeContent(w, req, name, modtime, content)
}

func (i *gatewayHandler) postHandler(w http.ResponseWriter, r *http.Request) {
	p, err := i.api.Unixfs().Add(r.Context(), files.NewReaderFile(r.Body))
	if err != nil {
		internalWebError(w, err)
		return
	}

	i.addUserHeaders(w) // ok, _now_ write user's headers.
	w.Header().Set("IPFS-Hash", p.Cid().String())
	http.Redirect(w, r, p.String(), http.StatusCreated)
}

func (i *gatewayHandler) putHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ds := i.api.Dag()

	// Parse the path
	rootCid, newPath, err := parseIpfsPath(r.URL.Path)
	if err != nil {
		webError(w, "WritableGateway: failed to parse the path", err, http.StatusBadRequest)
		return
	}
	if newPath == "" || newPath == "/" {
		http.Error(w, "WritableGateway: empty path", http.StatusBadRequest)
		return
	}
	newDirectory, newFileName := gopath.Split(newPath)

	// Resolve the old root.

	rnode, err := ds.Get(ctx, rootCid)
	if err != nil {
		webError(w, "WritableGateway: Could not create DAG from request", err, http.StatusInternalServerError)
		return
	}

	pbnd, ok := rnode.(*dag.ProtoNode)
	if !ok {
		webError(w, "Cannot read non protobuf nodes through gateway", dag.ErrNotProtobuf, http.StatusBadRequest)
		return
	}

	// Create the new file.
	newFilePath, err := i.api.Unixfs().Add(ctx, files.NewReaderFile(r.Body))
	if err != nil {
		webError(w, "WritableGateway: could not create DAG from request", err, http.StatusInternalServerError)
		return
	}

	newFile, err := ds.Get(ctx, newFilePath.Cid())
	if err != nil {
		webError(w, "WritableGateway: failed to resolve new file", err, http.StatusInternalServerError)
		return
	}

	// Patch the new file into the old root.

	root, err := mfs.NewRoot(ctx, ds, pbnd, nil)
	if err != nil {
		webError(w, "WritableGateway: failed to create MFS root", err, http.StatusBadRequest)
		return
	}

	if newDirectory != "" {
		err := mfs.Mkdir(root, newDirectory, mfs.MkdirOpts{Mkparents: true, Flush: false})
		if err != nil {
			webError(w, "WritableGateway: failed to create MFS directory", err, http.StatusInternalServerError)
			return
		}
	}
	dirNode, err := mfs.Lookup(root, newDirectory)
	if err != nil {
		webError(w, "WritableGateway: failed to lookup directory", err, http.StatusInternalServerError)
		return
	}
	dir, ok := dirNode.(*mfs.Directory)
	if !ok {
		http.Error(w, "WritableGateway: target directory is not a directory", http.StatusBadRequest)
		return
	}
	err = dir.Unlink(newFileName)
	switch err {
	case os.ErrNotExist, nil:
	default:
		webError(w, "WritableGateway: failed to replace existing file", err, http.StatusBadRequest)
		return
	}
	err = dir.AddChild(newFileName, newFile)
	if err != nil {
		webError(w, "WritableGateway: failed to link file into directory", err, http.StatusInternalServerError)
		return
	}
	nnode, err := root.GetDirectory().GetNode()
	if err != nil {
		webError(w, "WritableGateway: failed to finalize", err, http.StatusInternalServerError)
		return
	}
	newcid := nnode.Cid()

	i.addUserHeaders(w) // ok, _now_ write user's headers.
	w.Header().Set("IPFS-Hash", newcid.String())
	http.Redirect(w, r, gopath.Join(ipfsPathPrefix, newcid.String(), newPath), http.StatusCreated)
}

func (i *gatewayHandler) deleteHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// parse the path

	rootCid, newPath, err := parseIpfsPath(r.URL.Path)
	if err != nil {
		webError(w, "WritableGateway: failed to parse the path", err, http.StatusBadRequest)
		return
	}
	if newPath == "" || newPath == "/" {
		http.Error(w, "WritableGateway: empty path", http.StatusBadRequest)
		return
	}
	directory, filename := gopath.Split(newPath)

	// lookup the root

	rootNodeIPLD, err := i.api.Dag().Get(ctx, rootCid)
	if err != nil {
		webError(w, "WritableGateway: failed to resolve root CID", err, http.StatusInternalServerError)
		return
	}
	rootNode, ok := rootNodeIPLD.(*dag.ProtoNode)
	if !ok {
		http.Error(w, "WritableGateway: empty path", http.StatusInternalServerError)
		return
	}

	// construct the mfs root

	root, err := mfs.NewRoot(ctx, i.api.Dag(), rootNode, nil)
	if err != nil {
		webError(w, "WritableGateway: failed to construct the MFS root", err, http.StatusBadRequest)
		return
	}

	// lookup the parent directory

	parentNode, err := mfs.Lookup(root, directory)
	if err != nil {
		webError(w, "WritableGateway: failed to look up parent", err, http.StatusInternalServerError)
		return
	}

	parent, ok := parentNode.(*mfs.Directory)
	if !ok {
		http.Error(w, "WritableGateway: parent is not a directory", http.StatusInternalServerError)
		return
	}

	// delete the file

	switch parent.Unlink(filename) {
	case nil, os.ErrNotExist:
	default:
		webError(w, "WritableGateway: failed to remove file", err, http.StatusInternalServerError)
		return
	}

	nnode, err := root.GetDirectory().GetNode()
	if err != nil {
		webError(w, "WritableGateway: failed to finalize", err, http.StatusInternalServerError)
	}
	ncid := nnode.Cid()

	i.addUserHeaders(w) // ok, _now_ write user's headers.
	w.Header().Set("IPFS-Hash", ncid.String())
	// note: StatusCreated is technically correct here as we created a new resource.
	http.Redirect(w, r, gopath.Join(ipfsPathPrefix+ncid.String(), directory), http.StatusCreated)
}

func (i *gatewayHandler) addUserHeaders(w http.ResponseWriter) {
	for k, v := range i.config.Headers {
		w.Header()[k] = v
	}
}

func (i *gatewayHandler) cacheEntryFor(p string) (*ReedSolomonDirectory, bool, error) {
	v := i.rsDirs.Get(p)
	if v[0] == nil {
		return nil, false, nil
	}
	d, ok := v[0].(ReedSolomonDirectory)
	if !ok {
		return nil, false, fmt.Errorf("expected ReedSolomonDirectory cache item type")
	}
	return &d, true, nil
}

func (i *gatewayHandler) resolveAndGetFilesNode(
	ctx context.Context, w http.ResponseWriter, r *http.Request, pp ipath.Path, escPath string) (ipath.Resolved, files.Node, error) {
	// Resolve path to the final DAG node for the ETag
	resolvedPath, err := i.api.ResolvePath(ctx, pp)
	switch err {
	case nil:
	case coreiface.ErrOffline:
		webError(w, "btfs resolve -r "+escPath, err, http.StatusServiceUnavailable)
		return nil, nil, err
	default:
		webError(w, "btfs resolve -r "+escPath, err, http.StatusNotFound)
		return nil, nil, err
	}

	dr, err := i.api.Unixfs().Get(ctx, resolvedPath)
	if err != nil {
		webError(w, "btfs cat "+escPath, err, http.StatusNotFound)
		return nil, nil, err
	}

	return resolvedPath, dr, nil
}

func (i *gatewayHandler) isTopLevelEntryPath(r *http.Request, path string) (bool, error) {
	path = gopath.Clean(path)
	parts := strings.Split(path, "/")

	var (
		ipfpath *ipfspath.Path
		err     error
	)
	switch parts[1] {
	case "btfs":
		return len(parts) == 3 && parts[2] != "", nil
	case "btns":
		ipfpath, err = i.api.ResolveIpnsPath(r.Context(), ipath.New(path))
		if err == resolve.ErrNoNamesys {
			return false, coreiface.ErrOffline
		} else if err != nil {
			return false, err
		}
		return len(ipfpath.Segments()) == 2 && ipfpath.Segments()[1] != "", nil
	default:
		return false, fmt.Errorf("unsupported path namespace: [%s]", parts[1])
	}
}

func (i *gatewayHandler) setupEntityTag(
	w http.ResponseWriter, r *http.Request, resolvedPath ipath.Resolved, urlPath string) {
	etag := "\"" + resolvedPath.Cid().String() + "\""
	if r.Header.Get("If-None-Match") == etag || r.Header.Get("If-None-Match") == "W/"+etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	i.addUserHeaders(w) // ok, _now_ write user's headers.
	w.Header().Set("X-IPFS-Path", urlPath)
	w.Header().Set("Etag", etag)
}

func webError(w http.ResponseWriter, message string, err error, defaultCode int) {
	if _, ok := err.(resolver.ErrNoLink); ok {
		webErrorWithCode(w, message, err, http.StatusNotFound)
	} else if err == routing.ErrNotFound {
		webErrorWithCode(w, message, err, http.StatusNotFound)
	} else if err == context.DeadlineExceeded {
		webErrorWithCode(w, message, err, http.StatusRequestTimeout)
	} else {
		webErrorWithCode(w, message, err, defaultCode)
	}
}

func webErrorWithCode(w http.ResponseWriter, message string, err error, code int) {
	http.Error(w, fmt.Sprintf("%s: %s", message, err), code)
	if code >= 500 {
		log.Warningf("server error: %s: %s", err)
	}
}

// return a 500 error and log
func internalWebError(w http.ResponseWriter, err error) {
	webErrorWithCode(w, "internalWebError", err, http.StatusInternalServerError)
}

func getFilename(s string) string {
	if (strings.HasPrefix(s, ipfsPathPrefix) || strings.HasPrefix(s, ipnsPathPrefix)) && strings.Count(gopath.Clean(s), "/") <= 2 {
		// Don't want to treat ipfs.io in /ipns/ipfs.io as a filename.
		return ""
	}
	return gopath.Base(s)
}

func getDirRootPath(path string) (string, error) {
	path = gopath.Clean(path)
	idx := 1
	for j := 0; j < 2; j++ {
		idx += strings.Index(path[idx:], "/")
		if idx < 0 {
			return "", fmt.Errorf("root path can not be found from the given string [%s]", path)
		}
		idx++
	}
	return path[:idx-1], nil
}
