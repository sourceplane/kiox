package oci

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	digest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/sourceplane/kiox/internal/core"
	"github.com/sourceplane/kiox/internal/parser"
	"github.com/sourceplane/kiox/internal/state"
	"github.com/sourceplane/kiox/internal/ui/progress"
)

const layoutVersion = "1.0.0"

type imageIndex struct {
	SchemaVersion int                  `json:"schemaVersion"`
	MediaType     string               `json:"mediaType,omitempty"`
	Manifests     []ocispec.Descriptor `json:"manifests"`
}

func Pack(opts PackOptions) (PackResult, error) {
	pkg, err := parser.Load(opts.ManifestPath)
	if err != nil {
		return PackResult{}, err
	}
	defaultTool, ok := pkg.DefaultTool()
	if !ok {
		return PackResult{}, fmt.Errorf("provider %s does not declare a default tool", pkg.ProviderRef())
	}

	artifactRoot := opts.ArtifactRoot
	if artifactRoot == "" {
		artifactRoot = filepath.Dir(opts.ManifestPath)
	}
	tag := opts.Tag
	if tag == "" {
		tag = pkg.Provider.Metadata.Version
	}

	if err := os.RemoveAll(opts.OutputDir); err != nil {
		return PackResult{}, fmt.Errorf("reset output dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(opts.OutputDir, "blobs", "sha256"), 0o755); err != nil {
		return PackResult{}, fmt.Errorf("create output dir: %w", err)
	}

	manifestBytes, err := os.ReadFile(opts.ManifestPath)
	if err != nil {
		return PackResult{}, fmt.Errorf("read manifest file: %w", err)
	}
	configBytes, err := json.MarshalIndent(ProviderConfig{
		APIVersion:  pkg.APIVersion,
		Kind:        core.KindProvider,
		Namespace:   pkg.Provider.Metadata.Namespace,
		Name:        pkg.Provider.Metadata.Name,
		Version:     pkg.Provider.Metadata.Version,
		Description: pkg.Provider.Metadata.Description,
		Homepage:    pkg.Provider.Metadata.Homepage,
		License:     pkg.Provider.Metadata.License,
		Runtime:     defaultTool.Spec.Runtime.Type,
		Entrypoint:  defaultTool.PrimaryProvide(),
		DefaultTool: defaultTool.Metadata.Name,
	}, "", "  ")
	if err != nil {
		return PackResult{}, fmt.Errorf("marshal config: %w", err)
	}
	metadataBytes, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return PackResult{}, fmt.Errorf("marshal metadata: %w", err)
	}

	configDesc, err := writeBlob(opts.OutputDir, configBytes, MediaTypeConfig, nil)
	if err != nil {
		return PackResult{}, err
	}
	providerManifestDesc, err := writeBlob(opts.OutputDir, manifestBytes, MediaTypeManifest, map[string]string{"org.opencontainers.image.title": "kiox.yaml"})
	if err != nil {
		return PackResult{}, err
	}
	providerMetadataDesc, err := writeBlob(opts.OutputDir, metadataBytes, MediaTypeMetadata, map[string]string{"org.opencontainers.image.title": "package.json"})
	if err != nil {
		return PackResult{}, err
	}

	layers := []ocispec.Descriptor{providerManifestDesc, providerMetadataDesc}
	bundleNames := make([]string, 0, len(pkg.Bundles))
	for name := range pkg.Bundles {
		bundleNames = append(bundleNames, name)
	}
	sort.Strings(bundleNames)
	for _, bundleName := range bundleNames {
		bundle := pkg.Bundles[bundleName]
		for _, layer := range bundle.Spec.Layers {
			layerData, err := readBundleLayerData(artifactRoot, layer)
			if err != nil {
				return PackResult{}, err
			}
			desc, err := writeBlob(opts.OutputDir, layerData, bundleLayerMediaType(layer), map[string]string{
				"org.opencontainers.image.title": layer.Source,
				"io.kiox.bundle":                 bundleName,
				"io.kiox.platform":               layer.Platform.OS + "/" + layer.Platform.Arch,
				"io.kiox.source":                 layer.Source,
			})
			if err != nil {
				return PackResult{}, err
			}
			layers = append(layers, desc)
		}
	}

	imageManifestBytes, err := json.MarshalIndent(ImageManifest{
		SchemaVersion: 2,
		MediaType:     ocispec.MediaTypeImageManifest,
		ArtifactType:  ArtifactTypeProvider,
		Config:        configDesc,
		Layers:        layers,
		Annotations: map[string]string{
			"org.opencontainers.image.title":   pkg.ProviderRef(),
			"org.opencontainers.image.version": pkg.Provider.Metadata.Version,
		},
	}, "", "  ")
	if err != nil {
		return PackResult{}, fmt.Errorf("marshal image manifest: %w", err)
	}
	manifestDesc, err := writeBlob(opts.OutputDir, imageManifestBytes, ocispec.MediaTypeImageManifest, map[string]string{"org.opencontainers.image.ref.name": tag})
	if err != nil {
		return PackResult{}, err
	}

	indexBytes, err := json.MarshalIndent(imageIndex{SchemaVersion: 2, MediaType: ocispec.MediaTypeImageIndex, Manifests: []ocispec.Descriptor{manifestDesc}}, "", "  ")
	if err != nil {
		return PackResult{}, fmt.Errorf("marshal image index: %w", err)
	}
	if err := os.WriteFile(filepath.Join(opts.OutputDir, "index.json"), indexBytes, 0o644); err != nil {
		return PackResult{}, fmt.Errorf("write index.json: %w", err)
	}
	layoutBytes, err := json.Marshal(map[string]string{"imageLayoutVersion": layoutVersion})
	if err != nil {
		return PackResult{}, fmt.Errorf("marshal oci-layout: %w", err)
	}
	if err := os.WriteFile(filepath.Join(opts.OutputDir, "oci-layout"), layoutBytes, 0o644); err != nil {
		return PackResult{}, fmt.Errorf("write oci-layout: %w", err)
	}

	return PackResult{ProviderRef: pkg.ProviderRef(), Version: pkg.Provider.Metadata.Version, Tag: tag, LayoutDir: opts.OutputDir}, nil
}

func BinaryMediaType(goos, goarch string) string {
	return fmt.Sprintf("application/vnd.kiox.provider.binary.%s.%s.v1", goos, goarch)
}

func InstallMetadata(layoutPath, tag, activationHome, storeHome, alias string, out io.Writer) (state.ProviderMetadata, error) {
	tracker := progress.New(out)
	defer tracker.Finish()
	tracker.Step("lookup", "reading local OCI layout")
	return installMetadata(layoutPath, tag, activationHome, storeHome, alias, "", false, tracker)
}

func installMetadata(layoutPath, tag, activationHome, storeHome, alias, ref string, plainHTTP bool, tracker *progress.Tracker) (state.ProviderMetadata, error) {
	pkg, _, config, manifestBytes, metadataBytes, err := readLayout(layoutPath, tag)
	if err != nil {
		return state.ProviderMetadata{}, err
	}
	tracker.Done("lookup", fmt.Sprintf("resolved %s/%s@%s", config.Namespace, config.Name, config.Version))
	defaultTool, ok := pkg.DefaultTool()
	if !ok {
		return state.ProviderMetadata{}, fmt.Errorf("provider %s/%s@%s has no default tool", config.Namespace, config.Name, config.Version)
	}

	manifestDescriptor, err := resolveManifest(layoutPath, tag)
	if err != nil {
		return state.ProviderMetadata{}, err
	}
	storeID := providerStoreID(config, manifestDescriptor.Digest.String())
	storeRoot := state.StoreRoot(storeHome, storeID)
	storeLayoutPath := state.StoreLayoutPath(storeHome, storeID)
	if err := os.MkdirAll(storeLayoutPath, 0o755); err != nil {
		return state.ProviderMetadata{}, fmt.Errorf("create provider store: %w", err)
	}
	tracker.Step("cache", "preparing provider cache")
	if err := copyDirectory(layoutPath, storeLayoutPath); err != nil {
		return state.ProviderMetadata{}, fmt.Errorf("cache OCI layout: %w", err)
	}
	tracker.Info("cache", "cached OCI blobs")
	if err := os.WriteFile(filepath.Join(storeRoot, "kiox.yaml"), manifestBytes, 0o644); err != nil {
		return state.ProviderMetadata{}, fmt.Errorf("cache manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(storeRoot, "package.json"), metadataBytes, 0o644); err != nil {
		return state.ProviderMetadata{}, fmt.Errorf("cache provider metadata: %w", err)
	}

	meta := state.ProviderMetadata{
		Namespace:              config.Namespace,
		Name:                   config.Name,
		Version:                config.Version,
		StoreID:                storeID,
		StorePath:              storeRoot,
		Description:            config.Description,
		Homepage:               config.Homepage,
		License:                config.License,
		Runtime:                defaultTool.Spec.Runtime.Type,
		Entrypoint:             defaultTool.PrimaryProvide(),
		Capabilities:           defaultTool.CapabilityNames(),
		CapabilityDescriptions: capabilityDescriptions(defaultTool),
		Platforms:              toStatePlatforms(pkg.PlatformSummaries()),
		Source:                 state.Source{LayoutPath: storeLayoutPath, Tag: normalizedLayoutTag(ref, tag), Ref: resolvedSourceRef(ref, manifestDescriptor.Digest.String()), PlainHTTP: plainHTTP},
		InstalledAt:            time.Now().UTC(),
	}
	if err := saveStoreProviderMetadata(storeRoot, meta); err != nil {
		return state.ProviderMetadata{}, err
	}
	if err := state.SaveProviderMetadata(activationHome, meta); err != nil {
		return state.ProviderMetadata{}, err
	}
	tracker.Info("install", "persisted provider metadata")
	if err := state.SaveInstallSource(activationHome, meta.Namespace, meta.Name, meta.Version, meta.Source); err != nil {
		return state.ProviderMetadata{}, err
	}
	if alias != "" {
		aliases, err := state.LoadAliases(activationHome)
		if err != nil {
			return state.ProviderMetadata{}, err
		}
		aliases[alias] = state.MetadataKey(meta)
		if err := state.SaveAliases(activationHome, aliases); err != nil {
			return state.ProviderMetadata{}, err
		}
		tracker.Info("install", fmt.Sprintf("updated alias %s", alias))
	}
	tracker.Done("cache", "provider cached locally")
	return meta, nil
}

func MaterializeRuntime(meta state.ProviderMetadata, goos, goarch string, out io.Writer) (string, error) {
	pkg, err := LoadPackageModel(meta)
	if err != nil {
		return "", err
	}
	tool, ok := pkg.DefaultTool()
	if !ok {
		return "", fmt.Errorf("provider %s/%s@%s does not declare a default tool", meta.Namespace, meta.Name, meta.Version)
	}
	return MaterializeTool(meta, pkg, tool, goos, goarch, out)
}

func MaterializeTool(meta state.ProviderMetadata, pkg core.Package, tool core.Tool, goos, goarch string, out io.Writer) (string, error) {
	tracker := progress.New(out)
	defer tracker.Finish()
	return materializeTool(meta, pkg, tool, goos, goarch, true, tracker)
}

func ExpectedToolPath(meta state.ProviderMetadata, pkg core.Package, tool core.Tool, goos, goarch string) (string, error) {
	bundleName := strings.TrimSpace(tool.Spec.Source.Ref)
	bundle, ok := pkg.Bundles[bundleName]
	if !ok {
		return "", fmt.Errorf("source bundle %q not found", bundleName)
	}
	for _, layer := range bundle.Spec.Layers {
		if isArchiveMediaType(bundleLayerMediaType(layer)) {
			continue
		}
		if !platformMatches(layer.Platform, goos, goarch) {
			continue
		}
		return filepath.Join(state.MetadataStoreRoot(meta), filepath.FromSlash(layer.Source)), nil
	}
	return "", fmt.Errorf("tool %s does not publish %s/%s", tool.Metadata.Name, goos, goarch)
}

func materializeTool(meta state.ProviderMetadata, pkg core.Package, tool core.Tool, goos, goarch string, allowRemoteHydrate bool, tracker *progress.Tracker) (string, error) {
	tracker.Step("runtime", fmt.Sprintf("resolving %s/%s tool %s for %s/%s", meta.Namespace, meta.Name, tool.Metadata.Name, goos, goarch))
	_, view, _, _, _, err := readLayout(meta.Source.LayoutPath, meta.Source.Tag)
	if err != nil {
		return "", err
	}
	storeRoot := state.MetadataStoreRoot(meta)
	if storeRoot == "" {
		return "", fmt.Errorf("provider store root is missing for %s/%s@%s", meta.Namespace, meta.Name, meta.Version)
	}
	if err := os.MkdirAll(storeRoot, 0o755); err != nil {
		return "", fmt.Errorf("create provider store root: %w", err)
	}
	if err := extractBundleAssets(meta, view, storeRoot, goos, goarch, allowRemoteHydrate, tracker); err != nil {
		return "", err
	}
	bundleLayer, descriptor, ok := selectBundleLayer(pkg, view, tool, goos, goarch)
	if !ok {
		return "", fmt.Errorf("tool %s does not publish %s/%s", tool.Metadata.Name, goos, goarch)
	}
	binaryPath := filepath.Join(storeRoot, filepath.FromSlash(bundleLayer.Source))
	binaryName := filepath.Base(binaryPath)
	if exists(binaryPath) {
		tracker.Cached("runtime", fmt.Sprintf("cached %s", binaryName))
		return binaryPath, nil
	}
	tracker.Info("runtime", "extracting provider binary from cache")
	blob, err := readBlob(meta.Source.LayoutPath, descriptor)
	if err != nil {
		if allowRemoteHydrate && meta.Source.Ref != "" {
			tracker.Info("hydrate", "binary blob missing, hydrating from remote")
			if hydrateErr := hydrateStoreFromRemote(context.Background(), meta.Source.LayoutPath, meta.Source.Ref, meta.Source.PlainHTTP, remoteCopySelection{goos: goos, goarch: goarch}, tracker, ""); hydrateErr != nil {
				return "", err
			}
			return materializeTool(meta, pkg, tool, goos, goarch, false, tracker)
		}
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0o755); err != nil {
		return "", fmt.Errorf("create binary dir: %w", err)
	}
	if err := os.WriteFile(binaryPath, blob, 0o755); err != nil {
		return "", fmt.Errorf("write binary: %w", err)
	}
	tracker.Done("runtime", fmt.Sprintf("ready %s", binaryName))
	return binaryPath, nil
}

func capabilityDescriptions(tool core.Tool) map[string]string {
	if len(tool.Spec.Capabilities) == 0 {
		return nil
	}
	descriptions := make(map[string]string, len(tool.Spec.Capabilities))
	for name, capability := range tool.Spec.Capabilities {
		descriptions[name] = capability.Description
	}
	return descriptions
}

func copyCapabilityDescriptions(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	copyMap := make(map[string]string, len(values))
	for key, value := range values {
		copyMap[key] = value
	}
	return copyMap
}

func CurrentBinaryPath(meta state.ProviderMetadata) string {
	return filepath.Join(state.MetadataStoreRoot(meta), "bin", runtime.GOOS, runtime.GOARCH, meta.Entrypoint)
}

func providerStoreID(config ProviderConfig, manifestDigest string) string {
	material := strings.Join([]string{config.Namespace, config.Name, config.Version, manifestDigest}, "\x00")
	sum := sha256.Sum256([]byte(material))
	return hex.EncodeToString(sum[:])
}

func resolvedSourceRef(ref, manifestDigest string) string {
	trimmedRef := strings.TrimSpace(ref)
	if trimmedRef == "" {
		return ""
	}
	repository, _ := parseReference(trimmedRef)
	if strings.TrimSpace(manifestDigest) == "" {
		return trimmedRef
	}
	return repository + "@" + strings.TrimSpace(manifestDigest)
}

func readLayout(layoutPath, tag string) (core.Package, ProviderManifestView, ProviderConfig, []byte, []byte, error) {
	var pkg core.Package
	var view ProviderManifestView
	var config ProviderConfig
	var manifestBytes []byte
	var metadataBytes []byte

	manifestDescriptor, err := resolveManifest(layoutPath, tag)
	if err != nil {
		return pkg, view, config, nil, nil, err
	}
	manifestDoc, err := readBlob(layoutPath, manifestDescriptor)
	if err != nil {
		return pkg, view, config, nil, nil, err
	}
	var imageManifest ImageManifest
	if err := json.Unmarshal(manifestDoc, &imageManifest); err != nil {
		return pkg, view, config, nil, nil, fmt.Errorf("decode image manifest: %w", err)
	}
	configBlob, err := readBlob(layoutPath, imageManifest.Config)
	if err != nil {
		return pkg, view, config, nil, nil, err
	}
	if err := json.Unmarshal(configBlob, &config); err != nil {
		return pkg, view, config, nil, nil, fmt.Errorf("decode config blob: %w", err)
	}

	view = ProviderManifestView{ConfigDescriptor: imageManifest.Config, ManifestDescriptor: manifestDescriptor, BundleLayers: make([]BundleLayerDescriptor, 0, len(imageManifest.Layers))}
	for _, layer := range imageManifest.Layers {
		switch layer.MediaType {
		case MediaTypeManifest:
			view.ManifestDescriptor = layer
			manifestBytes, err = readBlob(layoutPath, layer)
			if err != nil {
				return pkg, view, config, nil, nil, err
			}
		case MediaTypeMetadata:
			view.MetadataDescriptor = layer
			metadataBytes, err = readBlob(layoutPath, layer)
			if err != nil {
				return pkg, view, config, nil, nil, err
			}
			if err := json.Unmarshal(metadataBytes, &pkg); err != nil {
				pkg = core.Package{}
			}
		default:
			view.BundleLayers = append(view.BundleLayers, descriptorFromLayer(layer))
		}
	}
	if pkg.Provider.Metadata.Name == "" {
		if len(manifestBytes) == 0 {
			return pkg, view, config, nil, nil, fmt.Errorf("provider manifest layer is missing")
		}
		pkg, err = parser.LoadBytes(manifestBytes)
		if err != nil {
			return pkg, view, config, nil, nil, err
		}
	}
	if len(metadataBytes) == 0 {
		metadataBytes, err = json.MarshalIndent(pkg, "", "  ")
		if err != nil {
			return pkg, view, config, nil, nil, fmt.Errorf("marshal normalized package: %w", err)
		}
	}
	view.BundleLayers = enrichBundleDescriptors(pkg, view.BundleLayers)
	return pkg, view, config, manifestBytes, metadataBytes, nil
}

func resolveManifest(layoutPath, tag string) (ocispec.Descriptor, error) {
	indexBytes, err := os.ReadFile(filepath.Join(layoutPath, "index.json"))
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("read index.json: %w", err)
	}
	var idx imageIndex
	if err := json.Unmarshal(indexBytes, &idx); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("decode index.json: %w", err)
	}
	if len(idx.Manifests) == 0 {
		return ocispec.Descriptor{}, fmt.Errorf("oci layout %s has no manifests", layoutPath)
	}
	if tag == "" {
		return idx.Manifests[0], nil
	}
	for _, descriptor := range idx.Manifests {
		if descriptor.Annotations["org.opencontainers.image.ref.name"] == tag {
			return descriptor, nil
		}
	}
	return ocispec.Descriptor{}, fmt.Errorf("tag %q not found in %s", tag, layoutPath)
}

func readBlob(layoutPath string, descriptor ocispec.Descriptor) ([]byte, error) {
	if descriptor.Digest.Algorithm() != digest.SHA256 {
		return nil, fmt.Errorf("unsupported digest algorithm %q", descriptor.Digest.Algorithm())
	}
	path := filepath.Join(layoutPath, "blobs", "sha256", descriptor.Digest.Encoded())
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read blob %s: %w", descriptor.Digest, err)
	}
	return data, nil
}

func writeBlob(layoutPath string, data []byte, mediaType string, annotations map[string]string) (ocispec.Descriptor, error) {
	hash := sha256.Sum256(data)
	hexDigest := hex.EncodeToString(hash[:])
	path := filepath.Join(layoutPath, "blobs", "sha256", hexDigest)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("write blob %s: %w", hexDigest, err)
	}
	return ocispec.Descriptor{MediaType: mediaType, Digest: digest.Digest("sha256:" + hexDigest), Size: int64(len(data)), Annotations: annotations}, nil
}

func tarDirectory(root, rel string) ([]byte, error) {
	var buffer bytes.Buffer
	tw := tar.NewWriter(&buffer)
	base := filepath.Join(root, rel)
	entries := make([]string, 0, 16)
	if err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == base {
			return nil
		}
		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entries = append(entries, filepath.ToSlash(relPath))
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Strings(entries)
	for _, entry := range entries {
		fullPath := filepath.Join(root, filepath.FromSlash(entry))
		info, err := os.Lstat(fullPath)
		if err != nil {
			return nil, err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return nil, err
		}
		header.Name = entry
		if info.IsDir() {
			header.Name += "/"
		}
		if err := tw.WriteHeader(header); err != nil {
			return nil, err
		}
		if info.IsDir() {
			continue
		}
		file, err := os.Open(fullPath)
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(tw, file); err != nil {
			file.Close()
			return nil, err
		}
		if err := file.Close(); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func extractTarBlob(layoutPath string, descriptor ocispec.Descriptor, targetDir string) error {
	blob, err := readBlob(layoutPath, descriptor)
	if err != nil {
		return err
	}
	tr := tar.NewReader(bytes.NewReader(blob))
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar entry: %w", err)
		}
		targetPath := filepath.Join(targetDir, filepath.FromSlash(header.Name))
		if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(targetDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid tar path %q", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return fmt.Errorf("create dir %s: %w", targetPath, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return fmt.Errorf("create parent dir %s: %w", targetPath, err)
			}
			file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("create file %s: %w", targetPath, err)
			}
			if _, err := io.Copy(file, tr); err != nil {
				file.Close()
				return fmt.Errorf("write file %s: %w", targetPath, err)
			}
			if err := file.Close(); err != nil {
				return fmt.Errorf("close file %s: %w", targetPath, err)
			}
		}
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func copyDirectory(srcDir, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if existing, err := os.ReadFile(target); err == nil {
			if bytes.Equal(existing, data) {
				return nil
			}
			if err := os.Remove(target); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

func descriptorFromLayer(layer ocispec.Descriptor) BundleLayerDescriptor {
	platform := core.PlatformSpec{}
	if raw := strings.TrimSpace(layer.Annotations["io.kiox.platform"]); raw != "" {
		parts := strings.SplitN(raw, "/", 2)
		if len(parts) == 2 {
			platform.OS = strings.TrimSpace(parts[0])
			platform.Arch = strings.TrimSpace(parts[1])
		}
	}
	return BundleLayerDescriptor{
		Bundle:     strings.TrimSpace(layer.Annotations["io.kiox.bundle"]),
		Platform:   platform,
		Source:     strings.TrimSpace(firstNonEmpty(layer.Annotations["io.kiox.source"], layer.Annotations["org.opencontainers.image.title"])),
		MediaType:  layer.MediaType,
		Descriptor: layer,
	}
}

func enrichBundleDescriptors(pkg core.Package, descriptors []BundleLayerDescriptor) []BundleLayerDescriptor {
	for index := range descriptors {
		candidateBundle, candidateLayer, ok := matchBundleLayer(pkg, descriptors[index])
		if !ok {
			continue
		}
		if strings.TrimSpace(descriptors[index].Bundle) == "" {
			descriptors[index].Bundle = candidateBundle
		}
		if strings.TrimSpace(descriptors[index].Source) == "" {
			descriptors[index].Source = candidateLayer.Source
		}
		if strings.TrimSpace(descriptors[index].Platform.OS) == "" {
			descriptors[index].Platform.OS = candidateLayer.Platform.OS
		}
		if strings.TrimSpace(descriptors[index].Platform.Arch) == "" {
			descriptors[index].Platform.Arch = candidateLayer.Platform.Arch
		}
	}
	return descriptors
}

func matchBundleLayer(pkg core.Package, descriptor BundleLayerDescriptor) (string, core.BundleLayer, bool) {
	for bundleName, bundle := range pkg.Bundles {
		for _, layer := range bundle.Spec.Layers {
			if strings.TrimSpace(descriptor.Source) != "" && descriptor.Source != layer.Source {
				continue
			}
			if strings.TrimSpace(descriptor.MediaType) != "" && bundleLayerMediaType(layer) != descriptor.MediaType {
				continue
			}
			if strings.TrimSpace(descriptor.Platform.OS) != "" && descriptor.Platform.OS != layer.Platform.OS {
				continue
			}
			if strings.TrimSpace(descriptor.Platform.Arch) != "" && descriptor.Platform.Arch != layer.Platform.Arch {
				continue
			}
			return bundleName, layer, true
		}
	}
	return "", core.BundleLayer{}, false
}

func readBundleLayerData(artifactRoot string, layer core.BundleLayer) ([]byte, error) {
	relSource := filepath.FromSlash(strings.TrimSpace(layer.Source))
	fullPath := filepath.Join(artifactRoot, relSource)
	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, fmt.Errorf("read bundle source %s: %w", layer.Source, err)
	}
	if info.IsDir() || isArchiveMediaType(bundleLayerMediaType(layer)) {
		return tarDirectory(artifactRoot, relSource)
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("read bundle source %s: %w", layer.Source, err)
	}
	return data, nil
}

func bundleLayerMediaType(layer core.BundleLayer) string {
	if mediaType := strings.TrimSpace(layer.MediaType); mediaType != "" {
		if mediaType == "application/vnd.kiox.tool.binary" {
			return BinaryMediaType(layer.Platform.OS, layer.Platform.Arch)
		}
		return mediaType
	}
	return BinaryMediaType(layer.Platform.OS, layer.Platform.Arch)
}

func selectBundleLayer(pkg core.Package, view ProviderManifestView, tool core.Tool, goos, goarch string) (core.BundleLayer, ocispec.Descriptor, bool) {
	bundleName := strings.TrimSpace(tool.Spec.Source.Ref)
	bundle, ok := pkg.Bundles[bundleName]
	if !ok {
		return core.BundleLayer{}, ocispec.Descriptor{}, false
	}
	for _, layer := range bundle.Spec.Layers {
		if isArchiveMediaType(bundleLayerMediaType(layer)) {
			continue
		}
		if !platformMatches(layer.Platform, goos, goarch) {
			continue
		}
		descriptor, ok := lookupBundleDescriptor(view, bundleName, layer)
		if ok {
			return layer, descriptor, true
		}
	}
	return core.BundleLayer{}, ocispec.Descriptor{}, false
}

func lookupBundleDescriptor(view ProviderManifestView, bundleName string, layer core.BundleLayer) (ocispec.Descriptor, bool) {
	for _, candidate := range view.BundleLayers {
		if strings.TrimSpace(candidate.Bundle) != "" && candidate.Bundle != bundleName {
			continue
		}
		if strings.TrimSpace(candidate.Source) != "" && candidate.Source != layer.Source {
			continue
		}
		if strings.TrimSpace(candidate.Platform.OS) != "" && candidate.Platform.OS != layer.Platform.OS {
			continue
		}
		if strings.TrimSpace(candidate.Platform.Arch) != "" && candidate.Platform.Arch != layer.Platform.Arch {
			continue
		}
		return candidate.Descriptor, true
	}
	return ocispec.Descriptor{}, false
}

func extractBundleAssets(meta state.ProviderMetadata, view ProviderManifestView, storeRoot, goos, goarch string, allowRemoteHydrate bool, tracker *progress.Tracker) error {
	for _, layer := range view.BundleLayers {
		if !isArchiveMediaType(layer.MediaType) {
			continue
		}
		if err := extractTarBlob(meta.Source.LayoutPath, layer.Descriptor, storeRoot); err != nil {
			if allowRemoteHydrate && meta.Source.Ref != "" {
				tracker.Info("hydrate", "cached asset layer missing, hydrating from remote")
				if hydrateErr := hydrateStoreFromRemote(context.Background(), meta.Source.LayoutPath, meta.Source.Ref, meta.Source.PlainHTTP, remoteCopySelection{goos: goos, goarch: goarch}, tracker, ""); hydrateErr != nil {
					return err
				}
				return extractBundleAssets(meta, view, storeRoot, goos, goarch, false, tracker)
			}
			return err
		}
	}
	return nil
}

func LoadPackageModel(meta state.ProviderMetadata) (core.Package, error) {
	var pkg core.Package
	storeRoot := state.MetadataStoreRoot(meta)
	if storeRoot == "" {
		return pkg, fmt.Errorf("provider store is missing for %s/%s@%s", meta.Namespace, meta.Name, meta.Version)
	}
	if pkg, ok := loadCachedPackageJSON(filepath.Join(storeRoot, "package.json")); ok {
		return pkg, nil
	}
	if layoutPath := strings.TrimSpace(meta.Source.LayoutPath); layoutPath != "" {
		layoutPkg, _, _, manifestBytes, metadataBytes, err := readLayout(layoutPath, meta.Source.Tag)
		if err == nil {
			backfillCachedPackageFiles(meta, storeRoot, manifestBytes, metadataBytes)
			return layoutPkg, nil
		}
	}
	manifestBytes, err := os.ReadFile(filepath.Join(storeRoot, "kiox.yaml"))
	if err != nil {
		return pkg, fmt.Errorf("read cached provider manifest: %w", err)
	}
	pkg, err = parser.LoadBytes(manifestBytes)
	if err != nil {
		return pkg, err
	}
	return pkg, nil
}

func loadCachedPackageJSON(path string) (core.Package, bool) {
	var pkg core.Package
	data, err := os.ReadFile(path)
	if err != nil {
		return pkg, false
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return core.Package{}, false
	}
	pkg.Normalize()
	if err := pkg.Validate(); err != nil {
		return core.Package{}, false
	}
	return pkg, true
}

func backfillCachedPackageFiles(meta state.ProviderMetadata, storeRoot string, manifestBytes, metadataBytes []byte) {
	if strings.TrimSpace(meta.StorePath) == "" {
		return
	}
	if len(metadataBytes) > 0 {
		writeMissingCacheFile(filepath.Join(storeRoot, "package.json"), metadataBytes)
	}
	if len(manifestBytes) > 0 {
		writeMissingCacheFile(filepath.Join(storeRoot, "kiox.yaml"), manifestBytes)
	}
}

func writeMissingCacheFile(path string, data []byte) {
	if len(data) == 0 {
		return
	}
	if _, err := os.Stat(path); err == nil || !os.IsNotExist(err) {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}

func platformMatches(platform core.PlatformSpec, goos, goarch string) bool {
	osMatch := platform.OS == goos || platform.OS == "any"
	archMatch := platform.Arch == goarch || platform.Arch == "any"
	return osMatch && archMatch
}

func isArchiveMediaType(mediaType string) bool {
	trimmed := strings.TrimSpace(mediaType)
	return strings.HasSuffix(trimmed, "+tar") || trimmed == MediaTypeAssets
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func toStatePlatforms(platforms []core.PlatformSummary) []state.PlatformSummary {
	out := make([]state.PlatformSummary, 0, len(platforms))
	for _, platform := range platforms {
		out = append(out, state.PlatformSummary{OS: platform.OS, Arch: platform.Arch})
	}
	return out
}
