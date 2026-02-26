#!/usr/bin/env node

const { spawn } = require("node:child_process");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");

function mapPlatformArch() {
  const platform = process.platform;
  const arch = process.arch;

  if (platform === "darwin") {
    if (arch === "arm64") return { goos: "darwin", assetArch: "arm64", ext: "" };
    if (arch === "x64") return { goos: "darwin", assetArch: "amd64", ext: "" };
  }

  if (platform === "linux") {
    if (arch === "arm64") return { goos: "linux", assetArch: "arm64", ext: "" };
    if (arch === "x64") return { goos: "linux", assetArch: "amd64", ext: "" };
    if (arch === "ia32") return { goos: "linux", assetArch: "386", ext: "" };
    if (arch === "arm") return { goos: "linux", assetArch: "armv7", ext: "" };
    if (arch === "ppc64") {
      if (os.endianness() === "LE") return { goos: "linux", assetArch: "ppc64le", ext: "" };
      return null;
    }
    if (arch === "s390x") return { goos: "linux", assetArch: "s390x", ext: "" };
    if (arch === "riscv64") return { goos: "linux", assetArch: "riscv64", ext: "" };
  }

  if (platform === "win32") {
    if (arch === "arm64") return { goos: "windows", assetArch: "arm64", ext: ".exe" };
    if (arch === "x64") return { goos: "windows", assetArch: "amd64", ext: ".exe" };
    if (arch === "ia32") return { goos: "windows", assetArch: "386", ext: ".exe" };
  }

  if (platform === "freebsd") {
    if (arch === "arm64") return { goos: "freebsd", assetArch: "arm64", ext: "" };
    if (arch === "x64") return { goos: "freebsd", assetArch: "amd64", ext: "" };
    if (arch === "ia32") return { goos: "freebsd", assetArch: "386", ext: "" };
    if (arch === "arm") return { goos: "freebsd", assetArch: "armv7", ext: "" };
  }

  if (platform === "openbsd") {
    if (arch === "arm64") return { goos: "openbsd", assetArch: "arm64", ext: "" };
    if (arch === "x64") return { goos: "openbsd", assetArch: "amd64", ext: "" };
  }

  return null;
}

function getVendorBinaryPath() {
  const mapped = mapPlatformArch();
  if (!mapped) return null;

  const packageRoot = path.resolve(__dirname, "..");
  const version = process.env.REALMS_APP_VERSION || require(path.join(packageRoot, "package.json")).version;
  const tag = version.startsWith("v") ? version : `v${version}`;

  return path.join(
    packageRoot,
    "vendor",
    `realms-app_${tag}_${mapped.goos}_${mapped.assetArch}${mapped.ext}`,
  );
}

function main() {
  const explicitBin = (process.env.REALMS_APP_BIN || "").trim();
  const binPath = explicitBin || getVendorBinaryPath();

  if (!binPath) {
    console.error(`[realms] 当前平台不受支持: ${process.platform}/${process.arch}`);
    process.exit(1);
  }

  if (!fs.existsSync(binPath)) {
    console.error("[realms] 未找到 realms-app 二进制。");
    console.error(`- 期望路径: ${binPath}`);
    console.error("- 可能原因: 安装时禁用了 npm scripts（postinstall 未执行），或下载失败。");
    console.error("- 修复: 重新安装（不要 --ignore-scripts），或手动运行 postinstall：");
    console.error(`  node "${path.join(path.resolve(__dirname, ".."), "scripts", "postinstall.js")}"`);
    process.exit(1);
  }

  const child = spawn(binPath, process.argv.slice(2), {
    stdio: "inherit",
    windowsHide: false,
  });

  child.on("exit", (code, signal) => {
    if (signal) process.kill(process.pid, signal);
    process.exit(code ?? 1);
  });
}

main();
