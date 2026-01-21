#!/usr/bin/env python3
# -*- coding: utf-8 -*-

from __future__ import annotations

import re
import sys
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[1]


def die(msg: str) -> None:
    print(f"ERROR: {msg}", file=sys.stderr)
    raise SystemExit(1)


def load_yaml(path: Path) -> dict:
    try:
        obj = yaml.safe_load(path.read_text(encoding="utf-8"))
    except Exception as e:
        die(f"解析 {path} 失败: {e}")
    if obj is None:
        return {}
    if not isinstance(obj, dict):
        die(f"{path} 顶层必须为 mapping")
    return obj


def extract_app_setting_keys(go_path: Path) -> list[str]:
    s = go_path.read_text(encoding="utf-8")
    # 支持 const 与 var 块；只提取字符串键名。
    keys = re.findall(r'\bSetting\w+\s*=\s*"([^"]+)"', s)
    return sorted(set(keys))


def main() -> None:
    config_path = ROOT / "config.example.yaml"
    go_path = ROOT / "internal/store/app_settings.go"
    tpl_path = ROOT / "internal/admin/templates/settings.html"

    cfg = load_yaml(config_path)
    defaults = cfg.get("app_settings_defaults")
    if not isinstance(defaults, dict):
        die("config.example.yaml 缺少顶层 app_settings_defaults 映射")

    all_keys = extract_app_setting_keys(go_path)
    required = sorted(
        {
            k
            for k in all_keys
            if k in {"site_base_url", "admin_time_zone"}
            or k.startswith("feature_disable_")
        }
    )
    missing = [k for k in required if k not in defaults]
    if missing:
        die("config.example.yaml 的 app_settings_defaults 缺少键: " + ", ".join(missing))

    tpl = tpl_path.read_text(encoding="utf-8")
    if "StartupConfigKeys" not in tpl:
        die("internal/admin/templates/settings.html 缺少 StartupConfigKeys 展示区块")

    print("OK: settings 同步校验通过")


if __name__ == "__main__":
    main()
