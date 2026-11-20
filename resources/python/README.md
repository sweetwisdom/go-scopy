# Python 脚本支持文件

## 默认使用本机 Python

发行包不携带 Python 解释器或第三方库，仅保留 SDK、示例、锁定依赖清单及本说明。

- 解释器优先级：界面/CLI 手动指定 > `GOSCRCPY_PYTHON` > PATH。
- Windows 默认查找 `python`，其他平台默认查找 `python3`，未找到时尝试另一命令。不自动使用旧版包残留的 runtime.json 或 runtime 目录。
- 在所选解释器中显式安装依赖，不修改其他 Python 环境：

```powershell
# 在发行包根目录执行；若选择了 python3 或自定义路径，下面也用同一解释器。
python -m pip install --require-hashes -r tools/python/requirements.lock
```

源码环境将清单路径替换为 `resources/python/requirements.lock`。软件运行和打包都不会自动联网安装依赖。

## 支持文件

- `lib/goscrcpy`：薄设备上下文层，返回原生 uiautomator2 对象，不重新定义选择器。
- `examples/device_info.py`：读取设备信息并截图，不主动点击。
- `requirements.in` / `requirements.lock`：uiautomator2 3.7.1 与依赖的锁定版本、哈希。
- SDK 添加到子进程的 PYTHONPATH，同时保留用户原有 PYTHONPATH；依赖使用所选解释器自身的 site-packages，不混入旧便携库。

## 开发者可选隔离环境

`scripts/prepare-python.py` 仍可用于开发测试：通过 uv 在本目录准备隔离 CPython 和依赖，不修改系统 Python。其生成的 runtime、runtime.json 与字节码不会进入发行包；也不会被运行器自动选择。需要使用时显式指定该解释器，并由开发者配置其依赖路径。

维护者更新依赖时应同步 requirements.in / requirements.lock 与运行器版本校验，不在用户运行任务时浮动升级。
