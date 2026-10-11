# MaaEnd-LingMoche

自用分支 `codex/personal-v2` 保留赠礼 PR #6311 的改动，并合并官方 `v2` 的更新。原赠礼分支保留作为开发记录。

## 更新来源

客户端通过 `https://github.com/LingMoche/MaaEnd` 的 GitHub Releases 检查更新。定制 MXU 不使用 MaaEnd 的 Mirror酱版本源；更新包包含定制客户端，因此后续安装更新仍保留这个来源。

第一次运行默认使用公测频道。正式频道只接受正式版本；公测频道接受正式版、beta 和 rc，并选择版本号最大的有效版本。此频道没有发布版本时会显示明确提示。

源码提交和 Actions 产物不会直接成为客户端更新。需将验证通过的完整 ZIP 发布到自己的 Release，发布标签与包内 `interface.json` 的版本必须一致，附件名称必须是 `MaaEnd-LingMoche-win-x86_64-<版本>.zip`。

## 持续合并官方更新

在干净的自用工作目录执行：

```powershell
git switch codex/personal-v2
git fetch origin
git merge --no-edit origin/v2
```

`origin` 是 MaaEnd/MaaEnd，`myfork` 是 LingMoche/MaaEnd。合并成功并确认功能后提交和推送到 `myfork`；不要对自用分支执行丢弃个人提交的同步操作。

## 构建与发布

`.github/workflows/lingmoche-build.yml` 在自用分支收到相关源码更新时构建 Windows x86_64 包。GUI 与原生 Agent 分开编译，最终步骤校验组件来源、资源加载、Agent 注册与离线导航查询。游戏实测需要另外执行。

每次发布前提高工作流的 `BUILD_VERSION`。公测版本示例 `v2.33.0-beta.2.lingmoche.1`，下一次可用 `v2.33.0-beta.2.lingmoche.2`；正式版使用标准版本如 `v2.33.1`。不要使用 ci、test 或 debug 版本，客户端会禁用这类版本的资源更新。

构建成功后下载 `MaaEnd-LingMoche-win-x86_64-<版本>` 产物；产物中包含可直接解压运行的 ZIP 和校验摘要。确认构建记录及所需游戏实测后，在自己的 Releases 上传 ZIP，公测版标为 pre-release。以后客户端的“检查更新”会从这里获取新版本。

普通的 `install.yml` 在此 fork 上停用，避免产生未包含定制更新器的包。请使用专用构建产物发布。

MXU 源码与补丁固定在本目录的 `mxu-upstream-commit.txt` 和 `mxu-github-updater.patch`。升级 GUI 基线时需要重新核对补丁和更新逻辑，保持自用更新能力。
