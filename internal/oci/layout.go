package oci

import (
	"archive/tar"
	"bytes"
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
	"gopkg.in/yaml.v3"

	"github.com/sourceplane/tinx/internal/manifest"
	"github.com/sourceplane/tinx/internal/state"
)

const layoutVersion = "1.0.0"

type imageIndex struct {
	SchemaVersion int                  `json:"schemaVersion"`
	MediaType     string               `json:"mediaType,omitempty"`
	Manifests     []ocispec.Descriptor `json:"manifests"`
}

func Pack(opts PackOptions) (PackResult, error) {
	provider, err := manifest.Load(opts.ManifestPath)
	if err != nil {
		return PackResult{}, err
	}

	artifactRoot := opts.ArtifactRoot
	if artifactRoot == "" {
		artifactRoot = filepath.Dir(opts.ManifestPath)
	}
	tag := opts.Tag
	if tag == "" {
		tag = provider.Metadata.Version
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
		APIVersion:  provider.APIVersion,
		Kind:        provider.Kind,
		Namespace:   provider.Metadata.Namespace,
		Name:        provider.Metadata.Name,
		Version:     provider.Metadata.Version,
		Description: provider.Metadata.Description,
		Homepage:    provider.Metadata.Homepage,
		License:     provider.Metadata.License,
		Runtime:     provider.Spec.Runtime,
		Entrypoint:  provider.Spec.Entrypoint,
	}, "", "  ")
	if err != nil {
		return PackResult{}, fmt.Errorf("marshal config: %w", err)
	}
	metadataBytes, err := json.MarshalIndent(ProviderMetadata{
		Namespace:    provider.Metadata.Namespace,
		Name:         provider.Metadata.Name,
		Version:      provider.Metadata.Version,
		Description:  provider.Metadata.Description,
		Entrypoint:   provider.Spec.Entrypoint,
		Runtime:      provider.Spec.Runtime,
		Capabilities: provider.CapabilityNames(),
		Platforms:    toMetadataPlatforms(provider.Spec.Platforms),
	}, "", "  ")
	if err != nil {
		return PackResult{}, fmt.Errorf("marshal metadata: %w", err)
	}

	configDesc, err := writeBlob(opts.OutputDir, configBytes, MediaTypeConfig, nil)
	if err != nil {
		return PackResult{}, err
	}
	providerManifestDesc, err := writeBlob(opts.OutputDir, manifestBytes, MediaTypeManifest, map[string]string{"org.opencontainers.image.title": "tinx.yaml"})
	if err != nil {
		return PackResult{}, err
	}
	providerMetadataDesc, err := writeBlob(opts.OutputDir, metadataBytes, MediaTypeMetadata, map[string]string{"org.opencontainers.image.title": "metadata.json"})
	if err != nil {
		return PackResult{}, err
	}

	layers := []ocispec.Descriptor{providerManifestDesc, providerMetadataDesc}
	if assetsRoot := provider.AssetsRoot(); assetsRoot != "" {
		assetsPath := filepath.Join(artifactRoot, filepath.FromSlash(assetsRoot))
		if exists(assetsPath) {
			assetsBytes, err := tarDirectory(artifactRoot, filepath.FromSlash(assetsRoot))
			if err != nil {
				return PackResult{}, fmt.Errorf("archive assets: %w", err)
			}
			assetsDesc, err := writeBlob(opts.OutputDir, assetsBytes, MediaTypeAssets, map[string]string{"org.opencontainers.image.title": assetsRoot})
			if err != nil {
				return PackResult{}, err
			}
			layers = append(layers, assetsDesc)
		}
	}
	for _, platform := range provider.Spec.Platforms {
		binaryPath := filepath.Join(artifactRoot, filepath.FromSlash(platform.Binary))
		binaryBytes, err := os.ReadFile(binaryPath)
		if err != nil {
			return PackResult{}, fmt.Errorf("read platform binary %s/%s: %w", platform.OS, platform.Arch, err)
		}
		desc, err := writeBlob(opts.OutputDir, binaryBytes, BinaryMediaType(platform.OS, platform.Arch), map[string]string{
			"org.opencontainers.image.title": platform.Binary,
			"io.tinx.platform":               platform.OS + "/" + platform.Arch,
		})
		if err != nil {
			return PackResult{}, err
		}
		layers = append(layers, desc)
	}

	imageManifestBytes, err := json.MarshalIndent(ImageManifest{
		SchemaVersion: 2,
		MediaType:     ocispec.MediaTypeImageManifest,
		ArtifactType:  ArtifactTypeProvider,
		Config:        configDesc,
		Layers:        layers,
		Annotations: map[string]string{
			"org.opencontainers.image.title":   provider.Ref(),
			"org.opencontainers.image.version": provider.Metadata.Version,
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

	return PackResult{ProviderRef: provider.Ref(), Version: provider.Metadata.Version, Tag: tag, LayoutDir: opts.OutputDir}, nil
}

func BinaryMediaType(goos, goarch string) string {
	return fmt.Sprintf("application/vnd.tinx.provider.binary.%s.%s.v1", goos, goarch)
}

func InstallMetadata(layoutPath, tag, home, alias string) (state.ProviderMetadata, error) {
	return installMetadata(layoutPath, tag, home, alias, "")
}

func installMetadata(layoutPath, tag, home, alias, ref string) (state.ProviderMetadata, error) {
	provider, _, config, metadata, manifestBytes, metadataBytes, err := readLayout(layoutPath, tag)
	if err != nil {
		return state.ProviderMetadata{}, err
	}

	versionRoot := state.VersionRoot(home, provider.Metadata.Namespace, provider.Metadata.Name, provider.Metadata.Version)
	if err := os.MkdirAll(versionRoot, 0o755); err != nil {
		return state.ProviderMetadata{}, fmt.Errorf("create version root: %w", err)
	}
	cachedLayoutPath := filepath.Join(versionRoot, "oci")
	if err := os.RemoveAll(cachedLayoutPath); err != nil {
		return state.ProviderMetadata{}, fmt.Errorf("reset cached layout: %w", err)
	}
	if err := copyDirectory(layoutPath, cachedLayoutPath); err != nil {
		return state.ProviderMetadata{}, fmt.Errorf("cache OCI layout: %w", err)
	}
	if err := os.WriteFile(filepath.Join(versionRoot, "tinx.yaml"), manifestBytes, 0o644); err != nil {
		return state.ProviderMetadata{}, fmt.Errorf("cache manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(versionRoot, "provider-metadata.json"), metadataBytes, 0o644); err != nil {
		return state.ProviderMetadata{}, fmt.Errorf("cache provider metadata: %w", err)
	}

	meta := state.ProviderMetadata{
		Namespace:    config.Namespace,
		Name:         config.Name,
		Version:      config.Version,
		Description:  config.Description,
		Homepage:     config.Homepage,
		License:      config.License,
		Runtime:      config.Runtime,
		Entrypoint:   config.Entrypoint,
		Capabilities: append([]string(nil), metadata.Capabilities...),
		Platforms:    toStatePlatforms(metadata.Platforms),
		Source:       state.Source{LayoutPath: cachedLayoutPath, Tag: tag, Ref: ref},
		InstalledAt:  time.Now().UTC(),
	}
	if err := state.SaveProviderMetadata(home, meta); err != nil {
		return state.ProviderMetadata{}, err
	}
	if err := state.SaveInstallSource(home, meta.Namespace, meta.Name, meta.Version, meta.Source); err != nil {
		return state.ProviderMetadata{}, err
	}
	if alias != "" {
		aliases, err := state.LoadAliases(home)
		if err != nil {
			return state.ProviderMetadata{}, err
		}
		aliases[alias] = meta.Namespace + "/" + meta.Name
		if err := state.SaveAliases(home, aliases); err != nil {
			return state.ProviderMetadata{}, err
		}
	}
	return meta, nil
}

func MaterializeRuntime(home string, meta state.ProviderMetadata, goos, goarch string) (string, error) {
	provider, view, _, _, _, _, err := readLayout(meta.Source.LayoutPath, meta.Source.Tag)
	if err != nil {
		return "", err
	}
	versionRoot := state.VersionRoot(home, meta.Namespace, meta.Name, meta.Version)
	if err := os.MkdirAll(versionRoot, 0o755); err != nil {
		return "", fmt.Errorf("create version root: %w", err)
	}
	if view.AssetsDescriptor != nil {
		if err := extractTarBlob(meta.Source.LayoutPath, *view.AssetsDescriptor, versionRoot); err != nil {
			return "", err
		}
	}
	platform, ok := provider.Platform(goos, goarch)
	if !ok {
		return "", fmt.Errorf("provider %s does not publish %s/%s", provider.Ref(), goos, goarch)
	}
	binaryDesc, ok := view.BinaryDescriptors[goos+"/"+goarch]
	if !ok {
		return "", fmt.Errorf("binary layer missing for %s/%s", goos, goarch)
	}
	binaryPath := filepath.Join(versionRoot, filepath.FromSlash(platform.Binary))
	if exists(binaryPath) {
		return binaryPath, nil
	}
	blob, err := readBlob(meta.Source.LayoutPath, binaryDesc)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0o755); err != nil {
		return "", fmt.Errorf("create binary dir: %w", err)
	}
	if err := os.WriteFile(binaryPath, blob, 0o755); err != nil {
		return "", fmt.Errorf("write binary: %w", err)
	}
	return binaryPath, nil
}

func CurrentBinaryPath(home string, meta state.ProviderMetadata) string {
	return filepath.Join(state.VersionRoot(home, meta.Namespace, meta.Name, meta.Version), "bin", runtime.GOOS, runtime.GOARCH, meta.Entrypoint)
}

func readLayout(layoutPath, tag string) (manifest.Provider, ProviderManifestView, ProviderConfig, ProviderMetadata, []byte, []byte, error) {
	var provider manifest.Provider
	var view ProviderManifestView
	var config ProviderConfig
	var metadata ProviderMetadata
	var manifestBytes []byte
	var metadataBytes []byte

	manifestDescriptor, err := resolveManifest(layoutPath, tag)
	if err != nil {
		return provider, view, config, metadata, nil, nil, err
	}
	manifestDoc, err := readBlob(layoutPath, manifestDescriptor)
	if err != nil {
		return provider, view, config, metadata, nil, nil, err
	}
	var imageManifest ImageManifest
	if err := json.Unmarshal(manifestDoc, &imageManifest); err != nil {
		return provider, view, config, metadata, nil, nil, fmt.Errorf("decode image manifest: %w", err)
	}
	configBlob, err := readBlob(layoutPath, imageManifest.Config)
	if err != nil {
		return provider, view, config, metadata, nil, nil, err
	}
	if err := json.Unmarshal(configBlob, &config); err != nil {
		return provider, view, config, metadata, nil, nil, fmt.Errorf("decode config blob: %w", err)
	}

	view = ProviderManifestView{ConfigDescriptor: imageManifest.Config, ManifestDescriptor: manifestDescriptor, BinaryDescriptors: map[string]ocispec.Descriptor{}}
	for _, layer := range imageManifest.Layers {
		switch layer.MediaType {
		case MediaTypeManifest:
			view.ManifestDescriptor = layer
			manifestBytes, err = readBlob(layoutPath, layer)
			if err != nil {
				return provider, view, config, metadata, nil, nil, err
			}
			if err := yaml.Unmarshal(manifestBytes, &provider); err != nil {
				return provider, view, config, metadata, nil, nil, fmt.Errorf("decode tinx manifest layer: %w", err)
			}
			if err := provider.Normalize(); err != nil {
				return provider, view, config, metadata, nil, nil, err
			}
		case MediaTypeMetadata:
			view.MetadataDescriptor = layer
			metadataBytes, err = readBlob(layoutPath, layer)
			if err != nil {
				return provider, view, config, metadata, nil, nil, err
			}
			if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
				return provider, view, config, metadata, nil, nil, fmt.Errorf("decode provider metadata layer: %w", err)
			}
		case MediaTypeAssets:
			layerCopy := layer
			view.AssetsDescriptor = &layerCopy
		default:
			if platformKey, ok := layer.Annotations["io.tinx.platform"]; ok {
				view.BinaryDescriptors[platformKey] = layer
			}
		}
	}
	return provider, view, config, metadata, manifestBytes, metadataBytes, nil
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
		return os.WriteFile(target, data, info.Mode())
	})
}

func toMetadataPlatforms(platforms []manifest.Platform) []ProviderPlatform {
	out := make([]ProviderPlatform, 0, len(platforms))
	for _, platform := range platforms {
		out = append(out, ProviderPlatform{OS: platform.OS, Arch: platform.Arch, Binary: platform.Binary})
	}
	return out
}

func toStatePlatforms(platforms []ProviderPlatform) []state.PlatformSummary {
	out := make([]state.PlatformSummary, 0, len(platforms))
	for _, platform := range platforms {
		out = append(out, state.PlatformSummary{OS: platform.OS, Arch: platform.Arch})
	}
	return out
}
