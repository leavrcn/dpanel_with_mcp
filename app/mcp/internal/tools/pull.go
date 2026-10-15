package tools

// dpanel_image_pull：MCP 侧本地执行的镜像拉取工具。
//
// 背景：面板 /app/image/tag-sync 的 pull 路径依赖 WebSocket progress fd
// （ws.NewFdProgressPip 从 gin context 取 userInfo.Fd），MCP 的 HTTP 回环调用
// 没有 ws 会话，必报 "fd not found"。本工具绕开面板 API，直接用 docker SDK
// 同步拉取（消费完整进度流后返回）。
//
// 依赖边界（刻意保持子模块可独立测试）：不 import 主仓 common/service/docker
// （它会传递拉进 containers/image → gpgme cgo / grpc v1.78，本机 go test 编不过），
// 也不 import app/application/logic（拉 gorm）。只依赖：
//   - docker/docker/client（纯 Go）
//   - registry-go-sdk（代理源回退，纯 Go）
//   - 主仓 common/function（ImageTag 解析，纯 Go）
// registry 源配置经面板 API /common/registry/get-list 获取（dispatcher 的 api client），
// 匹配逻辑复刻 logic.Image{}.GetRegistryConfig（纯函数部分）。
//
// docker client：MCP 运行在 dpanel 容器内，与面板共享 DOCKER_HOST / 默认 sock，
// client.NewClientWithOpts(client.FromEnv) 连接同一 daemon。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"github.com/distribution/reference"
	"github.com/docker/docker/api/types/image"
	dockerregistry "github.com/docker/docker/api/types/registry"
	dockerclient "github.com/docker/docker/client"
	"github.com/donknap/dpanel/common/function"
	dockerTypes "github.com/donknap/dpanel/common/service/docker/types"
	registrySdk "github.com/we7coreteam/registry-go-sdk"
)

// validatePullTag 用 reference.ParseNormalizedNamed 做真实校验：
// 拒绝空串、非法字符、多余段（busybox:latest:extra）等 daemon 才会报的底层错误。
func validatePullTag(tag string) error {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return errors.New("tag 不能为空")
	}
	if _, err := reference.ParseNormalizedNamed(tag); err != nil {
		return fmt.Errorf("tag 格式非法: %q（应为 repo[:tag] 形式）: %w", tag, err)
	}
	return nil
}

var (
	dockerClientOnce sync.Once
	dockerClient     *dockerclient.Client
	dockerClientErr  error
)

// getDockerClient 惰性创建 docker SDK client（FromEnv：DOCKER_HOST / 默认 sock）。
func getDockerClient() (*dockerclient.Client, error) {
	dockerClientOnce.Do(func() {
		dockerClient, dockerClientErr = dockerclient.NewClientWithOpts(
			dockerclient.FromEnv,
			dockerclient.WithAPIVersionNegotiation(),
		)
	})
	return dockerClient, dockerClientErr
}

// registryRow 面板 /common/registry/get-list 返回的镜像源行（只取需要的字段）。
type registryRow struct {
	ID            int32  `json:"id"`
	ServerAddress string `json:"serverAddress"`
	Setting       *struct {
		EnableHttp bool     `json:"enableHttp"`
		Proxy      []string `json:"proxy"`
		Username   string   `json:"username"`
		Password   string   `json:"password"` // get-list 为打码占位符，明文经 get-detail 获取
	} `json:"setting"`
}

// resolveRegistryConfig 复刻 logic.Image{}.GetRegistryConfig 的纯函数部分：
// 按镜像 registry 地址匹配面板镜像源（精确匹配或代理匹配），返回源配置。
// registryRows 来自面板 API（MCP dispatcher 的 api client 传入）。
func resolveRegistryConfig(registryUrl string, registryRows []registryRow) *dockerTypes.Registry {
	registryHost := registryUrl
	if registryAddress, err := function.ParseURL(registryUrl); err == nil {
		registryHost = registryAddress.Host
	}
	result := &dockerTypes.Registry{
		Address:      registryUrl,
		ProxyAddress: make([]string, 0),
	}

	var registryRowMatched *registryRow
	var proxyRegistryRow *registryRow
	for i := range registryRows {
		item := &registryRows[i]
		if item.Setting == nil {
			continue
		}
		sourceHost := item.ServerAddress
		if sourceAddress, err := function.ParseURL(item.ServerAddress); err == nil {
			sourceHost = sourceAddress.Host
		}
		if strings.EqualFold(registryHost, sourceHost) {
			registryRowMatched = item
			break
		}
		if proxyRegistryRow != nil {
			continue
		}
		for _, proxyAddress := range item.Setting.Proxy {
			if proxyURL, err := function.ParseURL(proxyAddress); err == nil && strings.EqualFold(registryHost, proxyURL.Host) {
				proxyRegistryRow = item
				break
			}
		}
	}

	firstProxyAddress := ""
	if registryRowMatched == nil && proxyRegistryRow != nil {
		registryRowMatched = proxyRegistryRow
		firstProxyAddress = registryUrl
	}
	if registryRowMatched == nil {
		return result
	}
	result.Address = registryRowMatched.ServerAddress
	result.EnableHttp = registryRowMatched.Setting.EnableHttp

	sourceHost := registryRowMatched.ServerAddress
	if sourceAddress, err := function.ParseURL(registryRowMatched.ServerAddress); err == nil {
		sourceHost = sourceAddress.Host
	}
	proxyAddresses := make([]string, 0, len(registryRowMatched.Setting.Proxy)+1)
	if firstProxyAddress != "" {
		proxyAddresses = append(proxyAddresses, firstProxyAddress)
	}
	for _, proxyAddress := range registryRowMatched.Setting.Proxy {
		proxyHost := ""
		proxyParsed := false
		if proxyURL, err := function.ParseURL(proxyAddress); err == nil {
			proxyHost = proxyURL.Host
			proxyParsed = true
		}
		if proxyParsed && strings.EqualFold(sourceHost, proxyHost) {
			continue
		}
		duplicate := false
		for _, existingAddress := range proxyAddresses {
			if proxyParsed {
				if existingURL, err := function.ParseURL(existingAddress); err == nil && strings.EqualFold(existingURL.Host, proxyHost) {
					duplicate = true
					break
				}
			}
			if existingAddress == proxyAddress {
				duplicate = true
				break
			}
		}
		if !duplicate {
			proxyAddresses = append(proxyAddresses, proxyAddress)
		}
	}
	result.ProxyAddress = proxyAddresses
	return result
}

// pullProgress 消费 docker pull 的 JSON 进度流直到 EOF（同步语义），
// 出现 ErrorDetail 时返回错误。对齐主仓 imageProgressReader.read 的错误判定。
func consumePullStream(ctx context.Context, reader io.ReadCloser) error {
	if reader == nil {
		return errors.New("docker image response stream is empty")
	}
	defer reader.Close()
	stopClose := context.AfterFunc(ctx, func() { _ = reader.Close() })
	defer stopClose()

	decoder := json.NewDecoder(reader)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var progress struct {
			ErrorDetail struct {
				Message string `json:"message"`
			} `json:"errorDetail"`
		}
		err := decoder.Decode(&progress)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		if progress.ErrorDetail.Message != "" {
			return errors.New(progress.ErrorDetail.Message)
		}
	}
}

// executeImagePull 本地执行镜像拉取：同步完成（消费完整进度流）。
// 镜像源配置经 dispatcher 的 api client 从面板读取（复用面板配置的国内代理源）。
func executeImagePull(ctx context.Context, payload map[string]any) (any, error) {
	rawTag, _ := payload["tag"].(string)
	if err := validatePullTag(rawTag); err != nil {
		return nil, err
	}
	platform, _ := payload["platform"].(string)

	client, err := getDockerClient()
	if err != nil {
		return nil, fmt.Errorf("docker client 创建失败: %w", err)
	}

	// 镜像源配置：经面板 API 读取（与面板 Web UI 的 pull 同源）。
	// 读取失败不阻塞 pull（退化为直连镜像原地址），但必须留日志可诊断。
	registryRows, err := loadRegistryRows(ctx)
	if err != nil {
		slog.Warn("image pull: load registry rows failed, fallback to direct pull", "error", err)
		registryRows = nil
	}
	imageNameDetail := function.ImageTag(rawTag)
	registryConfig := resolveRegistryConfig(imageNameDetail.Registry, registryRows)

	// 私有仓库凭据：匹配到面板镜像源行时经 get-detail 取明文（get-list 密码是打码占位符）
	if err := fillRegistryAuth(ctx, registryConfig, registryRows); err != nil {
		slog.Warn("image pull: load registry auth failed", "error", err)
	}

	// 代理源回退：对齐主仓 ImagePull 的尝试顺序
	proxyAddresses, sourceAddress := registryConfig.APIAddresses()
	originalTag := imageNameDetail.Uri()
	if tag, _, ok := strings.Cut(originalTag, "@"); ok {
		originalTag = tag
	}

	var lastErr error
	registryOptions := make([]registrySdk.Option, 0, len(proxyAddresses)+1)
	for _, address := range proxyAddresses {
		registryOptions = append(registryOptions, registrySdk.WithServer(address, "", ""))
	}
	registryOptions = append(registryOptions, registrySdk.WithRepository(imageNameDetail.BaseName, imageNameDetail.Version))

	for proxyServer := range registrySdk.New(registryOptions...).GetAvailableServers() {
		address := proxyServer.Url
		pullName := function.RegistryReference(address) + "/" + imageNameDetail.BaseName + ":" + imageNameDetail.Version
		out, err := client.ImagePull(ctx, pullName, image.PullOptions{Platform: platform})
		if err == nil {
			err = consumePullStream(ctx, out)
			if err == nil && pullName != originalTag {
				err = client.ImageTag(ctx, pullName, originalTag)
			}
			if err == nil {
				return pullResult(ctx, client, originalTag)
			}
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		lastErr = fmt.Errorf("pull image from %s: %w", address, err)
	}

	if sourceAddress != "" {
		pullName := function.RegistryReference(sourceAddress) + "/" + imageNameDetail.BaseName + ":" + imageNameDetail.Version
		out, err := client.ImagePull(ctx, pullName, image.PullOptions{Platform: platform, RegistryAuth: registryConfig.Auth})
		if err == nil {
			err = consumePullStream(ctx, out)
			if err == nil && pullName != originalTag {
				err = client.ImageTag(ctx, pullName, originalTag)
			}
			if err == nil {
				return pullResult(ctx, client, originalTag)
			}
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		lastErr = fmt.Errorf("pull image from %s: %w", sourceAddress, err)
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("image pull failed: no registry address")
}

// pullResult 拉取成功后读回本地 digest，给 agent 可验证的结果。
func pullResult(ctx context.Context, client *dockerclient.Client, tag string) (any, error) {
	result := map[string]any{
		"image": tag,
		"tag":   tag,
	}
	if inspect, err := client.ImageInspect(ctx, tag); err == nil && inspect.ID != "" {
		result["imageId"] = inspect.ID
	}
	return result, nil
}

// loadRegistryRows 经面板 API 读取镜像源列表，按 ID 升序返回
// （对齐主仓 dao.Registry.Order(ID.Asc())；面板 get-list 是 DESC，须排序）。
var loadRegistryRows = func(ctx context.Context) ([]registryRow, error) {
	if dispatcherAPIClient == nil {
		return nil, errors.New("api client 未注入")
	}
	data, err := dispatcherAPIClient.Post(ctx, "/common/registry/get-list", map[string]any{})
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		List []registryRow `json:"list"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	rows := envelope.List
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows, nil
}

// fillRegistryAuth 匹配到面板镜像源行时，经 get-detail 按 ID 取明文凭据，
// 生成 base64 auth 写入 registryConfig.Auth（对齐主仓 GetRegistryConfig 的 Auth 生成）。
// 匹配不到行或无凭据时静默跳过（匿名拉取）。
func fillRegistryAuth(ctx context.Context, registryConfig *dockerTypes.Registry, rows []registryRow) error {
	if dispatcherAPIClient == nil || registryConfig.Address == "" || len(rows) == 0 {
		return nil
	}
	// 找到与 registryConfig.Address 对应的行（resolveRegistryConfig 的匹配结果）
	var matched *registryRow
	for i := range rows {
		if rows[i].ServerAddress == registryConfig.Address {
			matched = &rows[i]
			break
		}
	}
	if matched == nil || matched.Setting == nil || matched.Setting.Username == "" || matched.ID == 0 {
		return nil
	}
	data, err := dispatcherAPIClient.Post(ctx, "/common/registry/get-detail", map[string]any{"id": matched.ID})
	if err != nil {
		return fmt.Errorf("get-detail %d: %w", matched.ID, err)
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var detail struct {
		Info registryRow `json:"info"`
	}
	if err := json.Unmarshal(raw, &detail); err != nil {
		return err
	}
	if detail.Info.Setting == nil || detail.Info.Setting.Username == "" || detail.Info.Setting.Password == "" {
		return errors.New("get-detail 未返回凭据")
	}
	auth, err := dockerregistry.EncodeAuthConfig(dockerregistry.AuthConfig{
		Username:      detail.Info.Setting.Username,
		Password:      detail.Info.Setting.Password,
		ServerAddress: registryConfig.Address,
	})
	if err != nil {
		return err
	}
	registryConfig.Auth = auth
	return nil
}

// dispatcherAPIClient 由 RegisterAll 注入（包级变量，避免 Execute 签名耦合 client）。
var dispatcherAPIClient apiPoster

// apiPoster 最小接口：只依赖 Post，避免 import cycle（client 包 import tools 会成环）。
type apiPoster interface {
	Post(ctx context.Context, path string, payload any) (any, error)
}

func init() {
	registerSlice([]ToolDef{
		{
			Name:        "dpanel_image_pull",
			Description: "拉取镜像到本地（同步完成，含面板镜像源代理回退与私有仓库凭据）。tag 形如 repo:tag。适用于更新 latest 标签镜像后再升级容器。注意：①仅作用于本机默认 Docker 环境（面板切换远程环境不影响本工具）；②大镜像拉取可能耗时数分钟",
			Params: []ParamDef{
				{Name: "tag", Type: "string", Description: "镜像 tag，如 ghcr.io/yetone/magpie:latest 或 busybox:latest", Required: true},
				{Name: "platform", Type: "string", Description: "目标平台（如 linux/amd64），留空用本机平台", Required: false},
			},
			Minimum: "read-write",
			// pull 只新增镜像层，不删除任何数据，不属破坏性操作
			Destructive: false,
			Execute:     executeImagePull,
		},
	})
}
