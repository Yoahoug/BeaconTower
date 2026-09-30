// 内网穿透客户端托管（doc/13 §14）：把「建好隧道还要手动去目标机跑 frpc」补成
// 面板直接部署与管理——经 SSH 在任意节点拉取镜像、写配置、起容器，并支持
// 同步配置/重启/停止/删除/日志。节点只需能 SSH 与能访问 GHCR。
package handler

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/frp"
	"github.com/Yoahoug/BeaconTower/internal/middleware"
	"github.com/Yoahoug/BeaconTower/internal/store"
	"github.com/gin-gonic/gin"
)

// FRP 客户端托管错误码（doc/04 错误码表的 2020 段）
const (
	frpCodeNode       = 2023 // 节点侧操作失败（SSH/Docker/容器）
	frpCodeNeedDocker = 2024 // 节点缺 Docker（可一键安装）
)

// 节点侧操作预算：装 Docker 要几分钟（apt-get update + 安装），部署/同步含拉镜像。
const (
	frpDockerTimeout = 12 * time.Minute
	frpDeployTimeout = 6 * time.Minute
	frpNodeTimeout   = 45 * time.Second
)

// failDeploy 把节点侧错误翻译成带处置建议的提示。
func failDeploy(c *gin.Context, err error) {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "未安装 Docker"):
		middleware.Fail(c, frpCodeNeedDocker, msg+"：可在下方勾选「自动安装 Docker」后重试")
	case strings.Contains(msg, "无 SSH 凭据"), strings.Contains(msg, "凭据解密失败"):
		middleware.Fail(c, 2010, msg)
	default:
		middleware.Fail(c, frpCodeNode, msg)
	}
}

// ---------- 视图 ----------

func (a *App) frpDeployView(dep *store.FRPDeploy) gin.H {
	platform, _ := a.DB.GetFRPPlatform(dep.PlatformID)
	server, _ := a.DB.GetServer(dep.ServerID)
	kind, pname, sname := "", "", ""
	if platform != nil {
		kind, pname = platform.Kind, platform.Name
	}
	if server != nil {
		sname = server.Name
	}
	tunnels := []gin.H{}
	all, _ := a.DB.ListFRPTunnels()
	byID := map[int64]*store.FRPTunnel{}
	for _, t := range all {
		byID[t.ID] = t
	}
	for _, id := range dep.TunnelIDList() {
		t := byID[id]
		if t == nil {
			tunnels = append(tunnels, gin.H{"id": id, "name": "（已删除）", "missing": true})
			continue
		}
		tunnels = append(tunnels, gin.H{
			"id": t.ID, "name": t.Name, "proto": t.Proto,
			"local_ip": t.LocalIP, "local_port": t.LocalPort, "remote": t.Remote,
		})
	}
	return gin.H{
		"id": dep.ID, "platform_id": dep.PlatformID, "platform_kind": kind, "platform_name": pname,
		"server_id": dep.ServerID, "server_name": sname,
		"tunnels": tunnels, "tunnel_ids": dep.TunnelIDList(),
		"image": dep.Image, "container": dep.Container, "config_path": dep.ConfigPath,
		"status": dep.Status, "last_error": dep.LastError, "docker_ver": dep.DockerVer,
		"dirty": dep.Dirty, "last_sync_at": dep.LastSyncAt,
		"created_at": dep.CreatedAt, "updated_at": dep.UpdatedAt,
	}
}

// wgLoadDeploy 取托管记录（带存在性校验）。
func (a *App) frpLoadDeploy(c *gin.Context) (*store.FRPDeploy, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		middleware.Fail(c, 1001, "参数错误：id 非法")
		return nil, false
	}
	dep, err := a.DB.GetFRPDeploy(id)
	if err != nil {
		middleware.Fail(c, 5000, err.Error())
		return nil, false
	}
	if dep == nil {
		middleware.Fail(c, 2002, "托管记录不存在")
		return nil, false
	}
	return dep, true
}

// frpTunnelsByIDs 取该平台下指定 id 的隧道（保持入参顺序，忽略不属于该平台的）。
func (a *App) frpTunnelsByIDs(platformID int64, ids []int64) ([]*store.FRPTunnel, error) {
	all, err := a.DB.ListFRPTunnels()
	if err != nil {
		return nil, err
	}
	byID := map[int64]*store.FRPTunnel{}
	for _, t := range all {
		if t.PlatformID == platformID {
			byID[t.ID] = t
		}
	}
	out := make([]*store.FRPTunnel, 0, len(ids))
	for _, id := range ids {
		if t := byID[id]; t != nil {
			out = append(out, t)
		}
	}
	return out, nil
}

// ---------- 接口 ----------

// FRPDeployList 托管列表（?refresh=1 时逐个回读节点上的真实容器状态）。
func (a *App) FRPDeployList(c *gin.Context) {
	list, err := a.DB.ListFRPDeploys()
	if err != nil {
		middleware.Fail(c, 5000, err.Error())
		return
	}
	if c.Query("refresh") == "1" {
		ctx, cancel := context.WithTimeout(c.Request.Context(), frpDeployTimeout)
		defer cancel()
		for _, dep := range list {
			if res, err := a.Deploy.Status(ctx, dep); err != nil {
				dep.Status, dep.LastError = "error", err.Error()
				_ = a.DB.SetFRPDeployResult(dep.ID, dep.Status, dep.LastError, dep.DockerVer, false, dep.Dirty)
			} else if res.Status != dep.Status {
				dep.Status = res.Status
				_ = a.DB.SetFRPDeployResult(dep.ID, dep.Status, dep.LastError, dep.DockerVer, false, dep.Dirty)
			}
		}
	}
	views := []gin.H{}
	for _, dep := range list {
		views = append(views, a.frpDeployView(dep))
	}
	middleware.OK(c, gin.H{"deployments": views})
}

type frpDeployInput struct {
	PlatformID    int64   `json:"platform_id"`
	ServerID      int64   `json:"server_id"`
	TunnelIDs     []int64 `json:"tunnel_ids"`
	InstallDocker bool    `json:"install_docker"`
	Image         string  `json:"image"`
}

// FRPDeployCreate 创建/更新托管并立刻部署（同一 (平台, 节点) 重复调用 = 覆盖式同步）。
func (a *App) FRPDeployCreate(c *gin.Context) {
	var in frpDeployInput
	if err := c.ShouldBindJSON(&in); err != nil {
		middleware.Fail(c, 1001, "参数错误：请求体须为 JSON")
		return
	}
	if in.PlatformID <= 0 || in.ServerID <= 0 {
		middleware.Fail(c, 1001, "参数错误：需要 platform_id 与 server_id")
		return
	}
	if len(in.TunnelIDs) == 0 {
		middleware.Fail(c, 1001, "请至少选择一条要在该节点上承载的隧道")
		return
	}
	platform, err := a.DB.GetFRPPlatform(in.PlatformID)
	if err != nil || platform == nil {
		middleware.Fail(c, 2002, "平台不存在或已解绑")
		return
	}
	server, err := a.DB.GetServer(in.ServerID)
	if err != nil || server == nil {
		middleware.Fail(c, 2002, "节点不存在")
		return
	}
	tunnels, err := a.frpTunnelsByIDs(in.PlatformID, in.TunnelIDs)
	if err != nil {
		middleware.Fail(c, 5000, err.Error())
		return
	}
	if len(tunnels) == 0 {
		middleware.Fail(c, 1001, "所选隧道不属于该平台或已被删除")
		return
	}
	if !a.deployOpLock(c) {
		return
	}
	defer a.deployOpUnlock()

	ctx, cancel := context.WithTimeout(c.Request.Context(), frpDockerTimeout)
	defer cancel()

	// 节点 Docker 就绪（必要时一键安装）
	st, err := a.Deploy.DockerCheck(ctx, in.ServerID)
	if err != nil {
		failDeploy(c, err)
		return
	}
	if !st.Present || !st.DaemonOK {
		if !in.InstallDocker {
			msg := "节点未安装 Docker"
			if st.Present {
				msg = "节点 Docker 守护进程不可用：" + st.Err
			}
			middleware.Fail(c, frpCodeNeedDocker, msg+"：勾选「自动安装 Docker」后可一键安装（apt/dnf/yum/apk 自动识别）")
			return
		}
		st, err = a.Deploy.InstallDocker(ctx, in.ServerID)
		if err != nil {
			failDeploy(c, fmt.Errorf("自动安装 Docker 失败: %w", err))
			return
		}
	}

	image := strings.TrimSpace(in.Image)
	if image == "" {
		image = a.Deploy.ImageFor(platform.Kind)
	}
	dep := &store.FRPDeploy{
		PlatformID: in.PlatformID, ServerID: in.ServerID,
		Image: image, Container: frp.ContainerName(platform.Kind, in.ServerID),
		ConfigPath: frp.ConfigPath(platform.Kind), DockerVer: st.Version,
	}
	dep.SetTunnelIDList(in.TunnelIDs)
	id, err := a.DB.UpsertFRPDeploy(dep)
	if err != nil {
		middleware.Fail(c, 5000, err.Error())
		return
	}
	dep.ID = id

	dctx, dcancel := context.WithTimeout(c.Request.Context(), frpDeployTimeout)
	defer dcancel()
	res, err := a.Deploy.Sync(dctx, dep, platform, tunnels)
	if err != nil {
		status := "error"
		if res != nil && res.Status != "" {
			status = res.Status
		}
		_ = a.DB.SetFRPDeployResult(dep.ID, status, err.Error(), st.Version, false, false)
		dep.Status, dep.LastError = status, err.Error()
		a.audit(a.actorOf(c), "frp_deploy", fmt.Sprintf("deploy:%d", dep.ID),
			fmt.Sprintf("节点 %s 部署失败: %v", server.Name, err), ipOf(c))
		failDeploy(c, err)
		return
	}
	_ = a.DB.SetFRPDeployResult(dep.ID, res.Status, "", res.DockerVer, true, false)
	dep.Status, dep.LastError, dep.DockerVer, dep.Dirty = res.Status, "", res.DockerVer, false
	a.audit(a.actorOf(c), "frp_deploy", fmt.Sprintf("deploy:%d", dep.ID),
		fmt.Sprintf("节点 %s 部署 %s 客户端（%d 条隧道）容器 %s", server.Name, platform.Name, len(tunnels), dep.Container), ipOf(c))
	middleware.OK(c, gin.H{"deployment": a.frpDeployView(dep), "log_tail": res.LogTail})
}

// FRPDeploySync 重新拉配置并重建容器（隧道增删改后的「同步配置」）。
func (a *App) FRPDeploySync(c *gin.Context) {
	dep, ok := a.frpLoadDeploy(c)
	if !ok {
		return
	}
	if !a.deployOpLock(c) {
		return
	}
	defer a.deployOpUnlock()
	platform, err := a.DB.GetFRPPlatform(dep.PlatformID)
	if err != nil || platform == nil {
		middleware.Fail(c, 2002, "平台不存在或已解绑")
		return
	}
	tunnels, err := a.frpTunnelsByIDs(dep.PlatformID, dep.TunnelIDList())
	if err != nil || len(tunnels) == 0 {
		middleware.Fail(c, 1001, "该托管包含的隧道已全部删除，请编辑或删除托管")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), frpDeployTimeout)
	defer cancel()
	res, err := a.Deploy.Sync(ctx, dep, platform, tunnels)
	if err != nil {
		status := "error"
		if res != nil && res.Status != "" {
			status = res.Status
		}
		_ = a.DB.SetFRPDeployResult(dep.ID, status, err.Error(), dep.DockerVer, false, dep.Dirty)
		a.audit(a.actorOf(c), "frp_deploy_sync", fmt.Sprintf("deploy:%d", dep.ID), "同步失败: "+err.Error(), ipOf(c))
		failDeploy(c, err)
		return
	}
	_ = a.DB.SetFRPDeployResult(dep.ID, res.Status, "", res.DockerVer, true, false)
	dep.Status, dep.LastError, dep.DockerVer, dep.Dirty = res.Status, "", res.DockerVer, false
	a.audit(a.actorOf(c), "frp_deploy_sync", fmt.Sprintf("deploy:%d", dep.ID),
		fmt.Sprintf("同步配置并重建容器 %s（%d 条隧道）", dep.Container, len(tunnels)), ipOf(c))
	middleware.OK(c, gin.H{"deployment": a.frpDeployView(dep), "log_tail": res.LogTail})
}

type frpDeployActionInput struct {
	Action string `json:"action"` // restart | stop | start
}

// FRPDeployAction 容器动作。
func (a *App) FRPDeployAction(c *gin.Context) {
	dep, ok := a.frpLoadDeploy(c)
	if !ok {
		return
	}
	var in frpDeployActionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		middleware.Fail(c, 1001, "参数错误：请求体须为 JSON")
		return
	}
	switch in.Action {
	case "restart", "stop", "start":
	default:
		middleware.Fail(c, 1001, "不支持的动作（restart/stop/start）")
		return
	}
	if !a.deployOpLock(c) {
		return
	}
	defer a.deployOpUnlock()
	ctx, cancel := context.WithTimeout(c.Request.Context(), frpNodeTimeout)
	defer cancel()
	res, err := a.Deploy.Action(ctx, dep, in.Action)
	if err != nil {
		_ = a.DB.SetFRPDeployResult(dep.ID, "error", err.Error(), dep.DockerVer, false, dep.Dirty)
		dep.Status, dep.LastError = "error", err.Error()
		a.audit(a.actorOf(c), "frp_deploy_action", fmt.Sprintf("deploy:%d", dep.ID), in.Action+" 失败: "+err.Error(), ipOf(c))
		failDeploy(c, err)
		return
	}
	_ = a.DB.SetFRPDeployResult(dep.ID, res.Status, "", dep.DockerVer, false, dep.Dirty)
	dep.Status, dep.LastError = res.Status, ""
	a.audit(a.actorOf(c), "frp_deploy_action", fmt.Sprintf("deploy:%d", dep.ID), in.Action+" → "+res.Status, ipOf(c))
	middleware.OK(c, gin.H{"deployment": a.frpDeployView(dep)})
}

// FRPDeployDelete 删除托管：先删节点上的容器与配置（含平台令牌），再删记录。
// ?keep=1 只删记录、保留节点上的容器（用户想自己接管时用）。
func (a *App) FRPDeployDelete(c *gin.Context) {
	dep, ok := a.frpLoadDeploy(c)
	if !ok {
		return
	}
	if !a.deployOpLock(c) {
		return
	}
	defer a.deployOpUnlock()
	keep := c.Query("keep") == "1"
	if !keep {
		ctx, cancel := context.WithTimeout(c.Request.Context(), frpNodeTimeout)
		defer cancel()
		if err := a.Deploy.Remove(ctx, dep, true); err != nil {
			failDeploy(c, err)
			return
		}
	}
	if err := a.DB.DeleteFRPDeploy(dep.ID); err != nil {
		middleware.Fail(c, 5000, err.Error())
		return
	}
	detail := "删除托管并清理节点容器/配置"
	if keep {
		detail = "删除托管记录（保留节点上的容器）"
	}
	a.audit(a.actorOf(c), "frp_deploy_delete", fmt.Sprintf("deploy:%d", dep.ID), detail, ipOf(c))
	middleware.OK(c, gin.H{"ok": true})
}

// FRPDeployLogs 容器日志（默认 200 行）。
func (a *App) FRPDeployLogs(c *gin.Context) {
	dep, ok := a.frpLoadDeploy(c)
	if !ok {
		return
	}
	tail, _ := strconv.Atoi(c.DefaultQuery("tail", "200"))
	ctx, cancel := context.WithTimeout(c.Request.Context(), frpNodeTimeout)
	defer cancel()
	logs, err := a.Deploy.Logs(ctx, dep, tail)
	if err != nil {
		failDeploy(c, err)
		return
	}
	middleware.OK(c, gin.H{"container": dep.Container, "logs": logs})
}

// FRPDeployStatus 回读节点上的真实容器状态并落库。
func (a *App) FRPDeployStatus(c *gin.Context) {
	dep, ok := a.frpLoadDeploy(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), frpNodeTimeout)
	defer cancel()
	res, err := a.Deploy.Status(ctx, dep)
	if err != nil {
		_ = a.DB.SetFRPDeployResult(dep.ID, "error", err.Error(), dep.DockerVer, false, dep.Dirty)
		dep.Status, dep.LastError = "error", err.Error()
		failDeploy(c, err)
		return
	}
	_ = a.DB.SetFRPDeployResult(dep.ID, res.Status, "", dep.DockerVer, false, dep.Dirty)
	dep.Status, dep.LastError = res.Status, ""
	middleware.OK(c, gin.H{"deployment": a.frpDeployView(dep)})
}

// FRPServerDocker 探测/安装某节点的 Docker（部署弹窗里选完节点即可先探一次）。
func (a *App) FRPServerDocker(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		middleware.Fail(c, 1001, "参数错误：id 非法")
		return
	}
	var in struct {
		Install bool `json:"install"`
	}
	_ = c.ShouldBindJSON(&in)
	if !a.deployOpLock(c) {
		return
	}
	defer a.deployOpUnlock()
	d := frpNodeTimeout
	if in.Install {
		d = frpDockerTimeout
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), d)
	defer cancel()
	var st *frp.DockerStatus
	if in.Install {
		st, err = a.Deploy.InstallDocker(ctx, id)
	} else {
		st, err = a.Deploy.DockerCheck(ctx, id)
	}
	if err != nil {
		if st != nil {
			middleware.OK(c, gin.H{"docker": st, "error": err.Error()})
			return
		}
		failDeploy(c, err)
		return
	}
	if in.Install {
		a.audit(a.actorOf(c), "frp_deploy_docker", fmt.Sprintf("server:%d", id), "安装 Docker: "+st.Version, ipOf(c))
	}
	middleware.OK(c, gin.H{"docker": st})
}

// deployOpLock 串行化托管写操作（同一时刻只跑一个节点侧动作）。
func (a *App) deployOpLock(c *gin.Context) bool {
	a.deployMu.Lock()
	return true
}

func (a *App) deployOpUnlock() { a.deployMu.Unlock() }
