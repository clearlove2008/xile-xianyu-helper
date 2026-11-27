#!/usr/bin/env python3
"""在 Windows/Linux 上交叉构建「喜乐闲鱼助手」macOS 通用安装包。

前置条件：
1. 已用 playwright-go 驱动按 mac14 / mac14-arm64 下载好 Chromium：
   PLAYWRIGHT_HOST_PLATFORM_OVERRIDE=mac14-arm64 PLAYWRIGHT_BROWSERS_PATH=<runtime>/arm64/playwright-browsers cli.js install chromium
   PLAYWRIGHT_HOST_PLATFORM_OVERRIDE=mac14       PLAYWRIGHT_BROWSERS_PATH=<runtime>/amd64/playwright-browsers cli.js install chromium
2. 已下载与驱动版本一致的 Node for macOS 官方 tarball：
   <workspace>/.macbuild/node/node-arm64.tar.xz
   <workspace>/.macbuild/node/node-x64.tar.xz
3. 已构建跨平台 pkg 工具：go build -o macospkg ./cmd/macospkg
   （github.com/deploymenttheory/go-macos-pkg，MIT）

产物：/Applications/喜乐闲鱼助手/喜乐闲鱼助手.app 的单包 .pkg，
同时包含 arm64 与 amd64 服务端及 Playwright runtime，由启动脚本按架构选择。
"""

from __future__ import annotations

import argparse
import os
import shutil
import subprocess
import sys
import tarfile
from pathlib import Path

APP_NAME = "喜乐闲鱼助手"
APP_DIR_NAME = "喜乐闲鱼助手"
APP_BUNDLE_NAME = f"{APP_NAME}.app"
IDENTIFIER = "com.xile.xianyu-helper"
LABEL = IDENTIFIER + ".server"
DATA_DIR_NAME = "XileXianyuHelper"
SERVICE_PORT = "59188"
ARCHES = ("arm64", "amd64")


def parse_args() -> argparse.Namespace:
    script_dir = Path(__file__).resolve().parent
    project_root = script_dir.parent.parent
    workspace = project_root.parent
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--project-root", type=Path, default=project_root)
    parser.add_argument("--workspace", type=Path, default=workspace)
    parser.add_argument("--version", default="2026.11.23")
    parser.add_argument(
        "--macospkg",
        type=Path,
        default=workspace / ".macbuild" / "tools" / "macospkg.exe",
    )
    parser.add_argument(
        "--driver-package",
        type=Path,
        default=project_root
        / "dist"
        / "windows"
        / "playwright-runtime"
        / "amd64"
        / "playwright-driver"
        / "package",
    )
    parser.add_argument(
        "--output",
        type=Path,
        default=None,
        help="默认输出到桌面的中文安装包文件名",
    )
    return parser.parse_args()


def write_text(path: Path, content: str, *, executable: bool = False) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content.lstrip("\n"), encoding="utf-8", newline="\n")
    if executable:
        path.chmod(0o755)


def copy_executable_tree(src: Path, dst: Path) -> None:
    if not src.is_dir():
        raise SystemExit(f"缺少运行时目录：{src}")
    shutil.copytree(src, dst, dirs_exist_ok=True)


def build_server(project_root: Path, stage: Path) -> None:
    env = dict(os.environ)
    env.update(CGO_ENABLED="0", GOOS="darwin")
    for arch in ARCHES:
        output = stage / "bin" / f"xianyu-server-{arch}"
        output.parent.mkdir(parents=True, exist_ok=True)
        env["GOARCH"] = arch
        print(f"[1/6] 编译 darwin/{arch} 服务端")
        subprocess.run(
            ["go", "build", "-o", str(output), "./cmd/server"],
            cwd=project_root,
            env=env,
            check=True,
        )


def extract_node(workspace: Path, stage: Path) -> dict[str, Path]:
    node_root = workspace / ".macbuild" / "node"
    result: dict[str, Path] = {}
    for arch, tarball_name, prefix in (
        ("arm64", "node-arm64.tar.xz", "node-v24.18.0-darwin-arm64"),
        ("amd64", "node-x64.tar.xz", "node-v24.18.0-darwin-x64"),
    ):
        tarball = node_root / tarball_name
        if not tarball.is_file():
            raise SystemExit(f"缺少 macOS Node 压缩包：{tarball}")
        destination = stage / "node" / arch
        if destination.exists():
            shutil.rmtree(destination)
        destination.mkdir(parents=True)
        print(f"[2/6] 解包 Node {arch}")
        with tarfile.open(tarball, "r:xz") as archive:
            archive.extractall(destination, filter="data")
        node = destination / prefix / "bin" / "node"
        if not node.is_file():
            raise SystemExit(f"Node 解包结果缺少可执行文件：{node}")
        result[arch] = node
    return result


def assemble_runtime(
    workspace: Path, stage: Path, driver_package: Path, nodes: dict[str, Path]
) -> None:
    if not driver_package.is_dir():
        raise SystemExit(f"缺少 Playwright driver 包：{driver_package}")
    for arch in ARCHES:
        runtime = stage / "runtime" / arch
        driver = runtime / "playwright-driver"
        browsers = runtime / "playwright-browsers"
        if driver.exists():
            shutil.rmtree(driver)
        driver.mkdir(parents=True)
        copy_executable_tree(driver_package, driver / "package")
        shutil.copy2(nodes[arch], driver / "node")
        browser_source = (
            workspace / ".macbuild" / "runtime" / arch / "playwright-browsers"
        )
        if browsers.exists():
            shutil.rmtree(browsers)
        copy_executable_tree(browser_source, browsers)
        links = browsers / ".links"
        if links.exists():
            shutil.rmtree(links)
        for marker in ("chromium-1228", "chromium_headless_shell-1228"):
            if not (browsers / marker).is_dir():
                raise SystemExit(f"{arch} 运行时缺少 {marker}，请先下载 macOS Chromium")
        print(f"[3/6] 组装 {arch} Playwright runtime")


def app_bundle_files(app: Path, version: str) -> None:
    contents = app / "Contents"
    for directory in (
        contents / "MacOS",
        contents / "Helpers",
        contents / "Resources",
    ):
        directory.mkdir(parents=True, exist_ok=True)

    write_text(
        contents / "Info.plist",
        f"""<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
\t<key>CFBundleIdentifier</key><string>{IDENTIFIER}</string>
\t<key>CFBundleName</key><string>{APP_NAME}</string>
\t<key>CFBundleDisplayName</key><string>{APP_NAME}</string>
\t<key>CFBundlePackageType</key><string>APPL</string>
\t<key>CFBundleVersion</key><string>{version}</string>
\t<key>CFBundleShortVersionString</key><string>{version}</string>
\t<key>CFBundleExecutable</key><string>{APP_NAME}</string>
\t<key>CFBundleIconFile</key><string>icon.icns</string>
\t<key>CFBundleIconName</key><string>AppIcon</string>
\t<key>LSUIElement</key><true/>
</dict>
</plist>
""",
    )

    write_text(
        contents / "Resources" / "symlinks.txt",
        "",
    )

    write_text(
        contents / "Resources" / "server.plist.template",
        f"""<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
\t<key>Label</key><string>{LABEL}</string>
\t<key>ProgramArguments</key>
\t<array>
\t\t<string>/bin/sh</string>
\t\t<string>__APP__/Contents/Helpers/start-server.sh</string>
\t</array>
\t<key>WorkingDirectory</key><string>__DATA_DIR__</string>
\t<key>RunAtLoad</key><true/>
\t<key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>
\t<key>StandardOutPath</key><string>__LOG_DIR__/server.log</string>
\t<key>StandardErrorPath</key><string>__LOG_DIR__/server.log</string>
</dict>
</plist>
""",
    )

    write_text(
        contents / "Helpers" / "start-server.sh",
        f"""#!/bin/sh
set -eu
APP="/Applications/{APP_DIR_NAME}/{APP_BUNDLE_NAME}"
case "$(uname -m)" in
  arm64) ARCH=arm64 ;;
  x86_64) ARCH=amd64 ;;
  *) ARCH=arm64 ;;
esac
exec "$APP/Contents/Helpers/xianyu-server-$ARCH" \\
  -addr 127.0.0.1:{SERVICE_PORT} \\
  -playwright-runtime-root "$APP/Contents/Resources/playwright-runtime"
""",
        executable=True,
    )

    write_text(
        contents / "MacOS" / APP_NAME,
        f"""#!/bin/sh
set -eu
LABEL="{LABEL}"
AGENT="$HOME/Library/LaunchAgents/$LABEL.plist"
UID_VALUE="$(id -u)"
if [ ! -f "$AGENT" ]; then
  echo "安装不完整：找不到 $AGENT，请重新安装喜乐闲鱼助手。" >&2
  exit 1
fi
if ! launchctl print "gui/$UID_VALUE/$LABEL" >/dev/null 2>&1; then
  launchctl bootstrap "gui/$UID_VALUE" "$AGENT" || true
fi
i=0
while [ "$i" -lt 20 ]; do
  if curl -fsS "http://127.0.0.1:{SERVICE_PORT}/health" >/dev/null 2>&1; then
    break
  fi
  i=$((i + 1))
  sleep 1
done
open "http://127.0.0.1:{SERVICE_PORT}/"
""",
        executable=True,
    )


def install_scripts(scripts: Path) -> None:
    write_text(
        scripts / "postinstall",
        f"""#!/bin/sh
set -eu
LOG="/var/log/xile-xianyu-helper-install.log"
exec >>"$LOG" 2>&1
trap 'printf "[%s] postinstall exit=%s\\n" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$?"' EXIT

APP="/Applications/{APP_DIR_NAME}/{APP_BUNDLE_NAME}"
PREFIX="/Applications/{APP_DIR_NAME}"
LABEL="{LABEL}"
CONSOLE_USER="$(stat -f '%Su' /dev/console)"
if [ "$CONSOLE_USER" = "root" ] || [ -z "$CONSOLE_USER" ]; then
  echo "没有登录用户，跳过 LaunchAgent 注册。"
  exit 0
fi
UID_VALUE="$(id -u "$CONSOLE_USER")"
HOME_DIR="$(dscl . -read "/Users/$CONSOLE_USER" NFSHomeDirectory | awk '{{print $2}}')"
DATA_DIR="$HOME_DIR/Library/Application Support/{DATA_DIR_NAME}"
LOG_DIR="$HOME_DIR/Library/Logs/{DATA_DIR_NAME}"
AGENT_DIR="$HOME_DIR/Library/LaunchAgents"
AGENT="$AGENT_DIR/$LABEL.plist"

xattr -dr com.apple.quarantine "$PREFIX" 2>/dev/null || true
chmod 0755 "$APP/Contents/MacOS/{APP_NAME}" "$APP/Contents/Helpers/start-server.sh"
chmod 0755 "$APP/Contents/Helpers/xianyu-server-arm64" "$APP/Contents/Helpers/xianyu-server-amd64"
find "$APP/Contents/Resources/playwright-runtime" -type f \\
  \\( -name node -o -name 'chrome-headless-shell' -o -name 'Google Chrome for Testing' -o -name 'ffmpeg-mac' -o -name 'ffmpeg-mac-arm64' \\) \\
  -exec chmod 0755 {{}} + 2>/dev/null || true
find "$APP/Contents/Resources/playwright-runtime" -type d -name '*.app' -exec chmod 0755 {{}} + 2>/dev/null || true

# Windows 打包环境无法创建 symlink；这里按清单恢复 Chromium Framework 内部链接。
SYMLINKS="$APP/Contents/Resources/symlinks.txt"
if [ -f "$SYMLINKS" ]; then
  while IFS="$(printf '\\t')" read -r relative target; do
    [ -n "$relative" ] || continue
    link_path="$APP/Contents/Resources/playwright-runtime/$relative"
    [ -e "$link_path" ] || continue
    if [ -L "$link_path" ]; then continue; fi
    rm -f "$link_path"
    ln -s "$target" "$link_path" 2>/dev/null || true
  done < "$SYMLINKS"
fi

mkdir -p "$DATA_DIR" "$LOG_DIR" "$AGENT_DIR"
chown -R "$CONSOLE_USER" "$DATA_DIR" "$LOG_DIR"

sed -e "s|__APP__|$APP|g" \\
    -e "s|__DATA_DIR__|$DATA_DIR|g" \\
    -e "s|__LOG_DIR__|$LOG_DIR|g" \\
    "$APP/Contents/Resources/server.plist.template" > "$AGENT"
chown "$CONSOLE_USER" "$AGENT"
chmod 0644 "$AGENT"

# 清理旧品牌或旧版服务，避免升级后两套服务同时监听。
for old_label in com.xile.xianyu-helper.server com.christ.xile-xianyu-helper.server; do
  launchctl bootout "gui/$UID_VALUE/$old_label" >/dev/null 2>&1 || true
  rm -f "$AGENT_DIR/$old_label.plist"
done
launchctl bootout "gui/$UID_VALUE/$LABEL" >/dev/null 2>&1 || true
launchctl bootstrap "gui/$UID_VALUE" "$AGENT"
""",
        executable=True,
    )


def helper_commands(folder: Path) -> None:
    write_text(
        folder / f"打开{APP_NAME}.command",
        f"""#!/bin/sh
set -eu
LABEL="{LABEL}"
UID_VALUE="$(id -u)"
AGENT="$HOME/Library/LaunchAgents/$LABEL.plist"
if [ -f "$AGENT" ] && ! launchctl print "gui/$UID_VALUE/$LABEL" >/dev/null 2>&1; then
  launchctl bootstrap "gui/$UID_VALUE" "$AGENT" || true
fi
i=0
while [ "$i" -lt 20 ]; do
  if curl -fsS "http://127.0.0.1:{SERVICE_PORT}/health" >/dev/null 2>&1; then break; fi
  i=$((i + 1)); sleep 1
done
open "http://127.0.0.1:{SERVICE_PORT}/"
""",
        executable=True,
    )
    write_text(
        folder / f"停止{APP_NAME}.command",
        f"""#!/bin/sh
set -eu
LABEL="{LABEL}"
UID_VALUE="$(id -u)"
launchctl bootout "gui/$UID_VALUE/$LABEL" >/dev/null 2>&1 || true
echo "已停止 {APP_NAME} 后台服务。需要恢复时双击「打开{APP_NAME}.command」。"
""",
        executable=True,
    )
    write_text(
        folder / f"卸载{APP_NAME}.command",
        f"""#!/bin/sh
set -eu
LABEL="{LABEL}"
UID_VALUE="$(id -u)"
launchctl bootout "gui/$UID_VALUE/$LABEL" >/dev/null 2>&1 || true
rm -f "$HOME/Library/LaunchAgents/$LABEL.plist"
rm -rf "/Applications/{APP_DIR_NAME}"
echo "{APP_NAME} 已卸载。账号与数据保留在 ~/Library/Application Support/{DATA_DIR_NAME}。"
""",
        executable=True,
    )


def main() -> None:
    args = parse_args()
    project_root: Path = args.project_root
    workspace: Path = args.workspace
    stage = project_root / "dist" / "macos" / "xile-cross"
    pkgroot = stage / "pkgroot"
    scripts = stage / "scripts"
    output: Path = args.output or (
        Path.home() / "Desktop" / f"{APP_NAME}-macOS通用版-v{args.version}.pkg"
    )
    output.parent.mkdir(parents=True, exist_ok=True)
    for path in (stage, pkgroot, scripts):
        if path.exists():
            shutil.rmtree(path)
        path.mkdir(parents=True)

    build_server(project_root, stage)
    nodes = extract_node(workspace, stage)
    assemble_runtime(workspace, stage, args.driver_package, nodes)
    print("[4/6] 组装 App Bundle")
    app = pkgroot / "Applications" / APP_DIR_NAME / APP_BUNDLE_NAME
    app_bundle_files(app, args.version)
    stage_bin = stage / "bin"
    for arch in ARCHES:
        shutil.copy2(
            stage_bin / f"xianyu-server-{arch}",
            app / "Contents" / "Helpers" / f"xianyu-server-{arch}",
        )
    for runtime_arch in ARCHES:
        runtime_src = stage / "runtime" / runtime_arch
        runtime_dst = app / "Contents" / "Resources" / "playwright-runtime" / runtime_arch
        copy_executable_tree(runtime_src, runtime_dst)
    symlink_manifest = workspace / ".macbuild" / "symlinks.txt"
    if not symlink_manifest.is_file():
        raise SystemExit(f"缺少 Playwright 符号链接清单：{symlink_manifest}")
    shutil.copy2(
        symlink_manifest, app / "Contents" / "Resources" / "symlinks.txt"
    )
    icon_root = project_root / "icon" / "macos"
    for icon_name in ("icon.icns", "Assets.car"):
        icon = icon_root / icon_name
        if not icon.is_file():
            raise SystemExit(f"缺少图标资源：{icon}")
        shutil.copy2(icon, app / "Contents" / "Resources" / icon_name)
    helper_commands(pkgroot / "Applications" / APP_DIR_NAME)
    install_scripts(scripts)

    print("[5/6] 生成 .pkg")
    if not args.macospkg.is_file():
        raise SystemExit(f"缺少 macospkg 工具：{args.macospkg}")
    command = [
        str(args.macospkg),
        "build",
        str(pkgroot),
        str(output),
        "--identifier",
        IDENTIFIER,
        "--version",
        args.version,
        "--scripts",
        str(scripts),
        "--compression",
        "gzip",
        "--ownership",
        "recommended",
        "--no-bundle-relocation",
        "--executable",
        r".*/MacOS/.*",
        "--executable",
        r".*/Helpers/.*",
        "--executable",
        r".*\.command$",
        "--executable",
        r".*/playwright-driver/node$",
        "--executable",
        r".*/chrome-headless-shell$",
        "--executable",
        r".*/Google Chrome for Testing$",
        "--executable",
        r".*/postinstall$",
    ]
    subprocess.run(command, check=True)

    print("[6/6] 校验安装包")
    if not output.is_file() or output.stat().st_size < 50 * 1024 * 1024:
        raise SystemExit(f"安装包异常：{output}")
    subprocess.run([str(args.macospkg), "info", str(output)], check=False)
    subprocess.run([str(args.macospkg), "list", str(output), "--scripts"], check=False)
    subprocess.run(
        [
            str(args.macospkg),
            "list",
            "-l",
            str(output),
            "--regexp",
            r"(Info\.plist|start-server\.sh|\.command|server\.plist\.template)$",
        ],
        check=False,
    )
    print(f"完成：{output}（{output.stat().st_size / 1024 / 1024:.1f} MiB）")


if __name__ == "__main__":
    sys.exit(main())
