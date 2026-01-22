#!/usr/bin/env python3
# -*- coding: utf-8 -*-

from __future__ import annotations

import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


def die(msg: str) -> None:
    print(f"ERROR: {msg}", file=sys.stderr)
    raise SystemExit(1)


def load_dotenv_keys(path: Path) -> set[str]:
    keys: set[str] = set()
    for idx, raw in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("export "):
            line = line[len("export ") :].strip()
        if "=" not in line:
            die(f"{path} 第 {idx} 行缺少 '='")
        key = line.split("=", 1)[0].strip()
        if not key:
            die(f"{path} 第 {idx} 行 key 为空")
        keys.add(key)
    return keys


def extract_app_setting_keys(go_path: Path) -> list[str]:
    s = go_path.read_text(encoding="utf-8")
    # 支持 const 与 var 块；只提取字符串键名。
    keys = re.findall(r'\bSetting\w+\s*=\s*"([^"]+)"', s)
    return sorted(set(keys))


def main() -> None:
    env_path = ROOT / ".env.example"
    go_path = ROOT / "internal/store/app_settings.go"
    tpl_path = ROOT / "internal/admin/templates/settings.html"

    env_keys = load_dotenv_keys(env_path)

    all_keys = extract_app_setting_keys(go_path)
    required = sorted(
        {
            k
            for k in all_keys
            if k in {"site_base_url", "admin_time_zone"}
            or k.startswith("feature_disable_")
        }
    )
    missing = [
        k
        for k in required
        if f"REALMS_APP_SETTINGS_DEFAULTS_{k.upper()}" not in env_keys
    ]
    if missing:
        die(".env.example 缺少 app_settings_defaults 对应变量: " + ", ".join(missing))

    tpl = tpl_path.read_text(encoding="utf-8")
    if "StartupConfigKeys" not in tpl:
        die("internal/admin/templates/settings.html 缺少 StartupConfigKeys 展示区块")

    print("OK: settings 同步校验通过")


if __name__ == "__main__":
    main()
