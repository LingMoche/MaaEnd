"""Verify and package the Windows MaaEnd-LingMoche build from CI outputs."""

import argparse
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import uuid
import zipfile
from pathlib import Path

import json5

REPOSITORY = "https://github.com/LingMoche/MaaEnd"
MXU_COMMIT = "9fa8cc51e8ff8cd89d99f3ea55fe3a7a82e6ede3"
MAAFW_VERSION = "v5.14.3"


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def run(*args: str, cwd: Path) -> str:
    return subprocess.check_output(
        args, cwd=cwd, text=True, encoding="utf-8", errors="replace"
    ).strip()


def save_json(path: Path, data: dict) -> None:
    path.write_text(
        json.dumps(data, ensure_ascii=False, indent=4) + "\n", encoding="utf-8"
    )


def check_version(version: str) -> None:
    if not re.fullmatch(
        r"v\d+\.\d+\.\d+(?:-(?:beta|rc)\.\d+(?:\.lingmoche\.\d+)?)?", version
    ):
        raise ValueError("Use vX.Y.Z or vX.Y.Z-beta.N.lingmoche.N")
    if any(marker in version.lower() for marker in ("ci", "test", "debug")):
        raise ValueError("MXU disables resource updates for ci/test version stamps")


def check_sources(repo: Path, install: Path, gui: Path) -> dict:
    source_commit = run("git", "rev-parse", "HEAD", cwd=repo)
    native = json.loads(
        (install / "BUILD_NATIVE_SOURCE.json").read_text(encoding="utf-8-sig")
    )
    frontend = json.loads(
        (gui / "BUILD_GUI_SOURCE.json").read_text(encoding="utf-8-sig")
    )
    patch = repo / "tools/lingmoche/mxu-github-updater.patch"
    if native["source_commit"] != source_commit:
        raise ValueError(
            "Native artifact and packaging checkout have different source commits"
        )
    if native["maafw_version"] != MAAFW_VERSION:
        raise ValueError("Unexpected MaaFramework build dependency")
    if frontend["mxu_commit"] != MXU_COMMIT:
        raise ValueError("GUI was not built from the pinned MXU source")
    if frontend["patch_sha256"].lower() != sha256(patch):
        raise ValueError("GUI updater patch does not match the packaging checkout")
    return {
        "repository": REPOSITORY,
        "source_commit": source_commit,
        "maafw_version": MAAFW_VERSION,
        "native": native,
        "gui": frontend,
        "updater_patch_sha256": sha256(patch),
    }


def check_resources(install: Path, interface: dict, report: dict):
    from maa.resource import Resource

    base_resource = None
    hashes = {}
    resource_items = interface.get("resource", [])
    if not resource_items:
        raise ValueError("interface.json has no resources")
    for item in resource_items:
        paths = item.get("path", [])
        if not paths:
            raise ValueError(f"Resource {item['name']} has no bundle paths")
        resource = Resource()
        for relative in paths:
            path = (install / relative).resolve()
            if not path.is_relative_to(install) or not path.is_dir():
                raise ValueError(f"Invalid resource path: {relative}")
            if not resource.post_bundle(path).wait().succeeded:
                raise RuntimeError(f"Resource bundle failed to load: {relative}")
        value = resource.hash
        if not value:
            raise RuntimeError(f"Empty resource hash: {item['name']}")
        item["hash"] = value
        hashes[item["name"]] = value
        if base_resource is None:
            base_resource = resource

    # Controller overlays must load with the same base resources as the application.
    overlays = {}
    for controller in interface.get("controller", []):
        paths = controller.get("attach_resource_path", [])
        if not paths:
            continue
        resource = Resource()
        for relative in [*resource_items[0]["path"], *paths]:
            path = (install / relative).resolve()
            if not path.is_relative_to(install) or not path.is_dir():
                raise ValueError(f"Invalid controller resource path: {relative}")
            if not resource.post_bundle(path).wait().succeeded:
                raise RuntimeError(f"Controller resource failed to load: {relative}")
        if not resource.hash:
            raise RuntimeError(f"Empty controller resource hash: {controller['name']}")
        overlays[controller["name"]] = resource.hash
    report["checks"]["resource_loading"] = {
        "status": "passed",
        "hashes": hashes,
        "controller_overlays": overlays,
    }
    return base_resource


def check_navigation(resource, client, install: Path, repo: Path) -> dict:
    import numpy as np
    from maa.controller import CustomController
    from maa.tasker import Tasker

    class OfflineController(CustomController):
        def connect(self):
            return True

        def request_uuid(self):
            return "maaend-lingmoche-build-verification"

        def screencap(self):
            return np.zeros((720, 1280, 3), dtype=np.uint8)

        def deny_input(self, *args):
            return False

        start_app = stop_app = click = swipe = touch_down = touch_move = touch_up = (
            deny_input
        )
        click_key = input_text = key_down = key_up = scroll = relative_move = deny_input

        def shell(self, *args):
            return None

    cases = json5.loads(
        (repo / "tests/MapNavmeshConnectivity/navmesh_connectivity.json").read_text(
            encoding="utf-8"
        )
    )
    case = cases["cases"][0]
    controller = OfflineController()
    if not controller.post_connection().wait().succeeded:
        raise RuntimeError("Offline controller failed to initialize")
    tasker = Tasker()
    if not tasker.bind(resource, controller) or not tasker.inited:
        raise RuntimeError("Offline navigation tasker failed to initialize")
    if not client.set_timeout(60_000):
        raise RuntimeError("Cannot set navigation verification timeout")
    navmesh = install / "resource/model/map/navmesh/base.nav.gz"
    if not navmesh.is_file():
        raise FileNotFoundError(navmesh)
    target = {"action": "NAVMESH", "target": case["goal"]}
    if "goal_tier" in case:
        target["target_tier"] = case["goal_tier"]
    if "goal_deck_y" in case:
        target["target_deck_y"] = case["goal_deck_y"]
    payload = {
        "op": "route_preview",
        "navmesh_file": str(navmesh),
        "position": case["start"],
        "position_zone": case["zone_id"],
        "custom_action_param": {
            "path": [{"action": "ZONE", "zone_id": case["zone_id"]}, target]
        },
    }
    node = "LingMocheBuildNavmeshQuery"
    harness = {
        node: {
            "recognition": "Custom",
            "custom_recognition": "MapNavmeshQuery",
            "custom_recognition_param": payload,
            "action": "DoNothing",
            "next": [],
            "pre_delay": 0,
            "post_delay": 0,
            "rate_limit": 0,
        }
    }
    job = tasker.post_task(node, harness).wait()
    detail = tasker.get_task_detail(job.job_id)
    result_node = detail.nodes[0] if detail and detail.nodes else None
    best = (
        result_node.recognition.best_result
        if result_node and result_node.recognition
        else None
    )
    result = best.detail if best else None
    if isinstance(result, (str, bytes)):
        result = json.loads(result)
    if not job.succeeded or not isinstance(result, dict) or not result.get("ok"):
        raise RuntimeError(f"Packaged navmesh route query failed: {result}")
    return {
        "status": "passed",
        "case": case["name"],
        "detail": result,
        "navmesh_sha256": sha256(navmesh),
    }


def check_agents(
    install: Path, repo: Path, resource, checks_dir: Path, report: dict
) -> None:
    from maa.agent_client import AgentClient

    required = {
        "go-service": {
            "action": {"GiftOperatorSessionAction"},
            "recognition": {
                "GiftOperatorCandidateRecognition",
                "GiftOperatorDialogueRecognition",
                "GiftOperatorGiftStatusRecognition",
                "GiftOperatorGiftSelectionRecognition",
            },
        },
        "cpp-algo": {
            "action": {"MapNavigateAction"},
            "recognition": {"MapNavmeshQuery", "MapLocateAssertLocation"},
        },
    }
    env = os.environ.copy()
    env["PATH"] = os.pathsep.join(
        [str(install / "agent"), str(install / "maafw"), env.get("PATH", "")]
    )
    report["checks"]["agents"] = {}
    for name, expected in required.items():
        identifier = f"lingmoche-build-{name}-{uuid.uuid4().hex}"
        client = AgentClient(identifier)
        process = None
        log_path = checks_dir / f"{name}.log"
        with log_path.open("w", encoding="utf-8") as log:
            try:
                if not client.bind(resource) or not client.set_timeout(30_000):
                    raise RuntimeError(f"Cannot initialize {name} client")
                process = subprocess.Popen(
                    [str(install / "agent" / f"{name}.exe"), identifier],
                    cwd=install,
                    env=env,
                    stdout=log,
                    stderr=subprocess.STDOUT,
                    creationflags=subprocess.CREATE_NO_WINDOW,
                )
                if not client.connect() or process.poll() is not None:
                    raise RuntimeError(f"{name} failed to connect; see {log_path}")
                actions = set(client.custom_action_list)
                recognitions = set(client.custom_recognition_list)
                if (
                    expected["action"] - actions
                    or expected["recognition"] - recognitions
                ):
                    raise RuntimeError(
                        f"{name} is missing expected custom registrations"
                    )
                report["checks"]["agents"][name] = {
                    "status": "passed",
                    "actions": sorted(actions),
                    "recognitions": sorted(recognitions),
                }
                if name == "cpp-algo":
                    report["checks"]["navmesh"] = check_navigation(
                        resource, client, install, repo
                    )
            finally:
                if process is not None:
                    if process.poll() is None:
                        process.terminate()
                    try:
                        process.wait(timeout=5)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        process.wait(timeout=5)
                client.disconnect()


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--install", type=Path, default=Path("install"))
    parser.add_argument("--gui", type=Path, default=Path("compiled-gui"))
    parser.add_argument("--version", required=True)
    parser.add_argument("--output", type=Path, default=Path("artifacts"))
    parser.add_argument("--checks", type=Path, default=Path("build-checks"))
    args = parser.parse_args()
    check_version(args.version)
    repo = Path(__file__).resolve().parents[2]
    install, gui = args.install.resolve(), args.gui.resolve()
    args.output.mkdir(parents=True, exist_ok=True)
    args.checks.mkdir(parents=True, exist_ok=True)

    report = check_sources(repo, install, gui)
    if report["native"]["build_version"] != args.version:
        raise ValueError(
            "Native build version does not match the requested package version"
        )
    for directory in ("config", "debug"):
        if (install / directory).exists():
            raise ValueError(f"Build install directory already contains {directory}")
    report["version"] = args.version
    report["platform"] = "win-x86_64"
    report["checks"] = {
        "game_execution": {
            "status": "not_run",
            "reason": "Build verification uses an offline controller",
        }
    }
    try:
        interface = json5.loads(
            (install / "interface.json").read_text(encoding="utf-8")
        )
        if (
            interface.get("github") != REPOSITORY
            or interface.get("name") != "MaaEnd-LingMoche"
        ):
            raise ValueError("interface.json does not identify the LingMoche fork")
        if "mirrorchyan_rid" in interface or "mirrorchyan_multiplatform" in interface:
            raise ValueError("Fork package must use its GitHub releases for updates")
        shutil.copy2(gui / "MaaEnd.exe", install / "MaaEnd.exe")
        shutil.copy2(gui / "MXU-LICENSE.txt", install / "MXU-LICENSE.txt")
        for relative in [
            "MaaEnd.exe",
            "agent/go-service.exe",
            "agent/cpp-algo.exe",
            "agent/WebView2Loader.dll",
            "maafw/MaaFramework.dll",
            "maafw/MaaAgentServer.dll",
        ]:
            path = install / relative
            if not path.is_file() or not path.stat().st_size:
                raise FileNotFoundError(
                    f"Required runtime file is absent or empty: {relative}"
                )

        for metadata in install.rglob(".git"):
            if metadata.is_file():
                metadata.unlink()
            elif metadata.is_dir():
                shutil.rmtree(metadata)

        os.environ["MAAFW_BINARY_PATH"] = str(install / "maafw")
        from maa.library import Library
        from maa.toolkit import Toolkit

        Library.open(install / "maafw")
        version = Library.version()
        if version.lstrip("v") != MAAFW_VERSION.lstrip("v"):
            raise RuntimeError(f"Unexpected runtime MaaFramework version: {version}")
        report["runtime_framework_version"] = version
        Toolkit.init_option(args.checks.resolve())
        resource = check_resources(install, interface, report)
        check_agents(install, repo, resource, args.checks.resolve(), report)
        interface["version"] = args.version
        save_json(install / "interface.json", interface)
        report["checks"]["gui_build"] = {
            "status": "passed",
            "method": "Patched MXU source compiled by the GUI job",
        }
        report["components"] = {
            relative: {"sha256": sha256(install / relative)}
            for relative in [
                "MaaEnd.exe",
                "agent/go-service.exe",
                "agent/cpp-algo.exe",
                "maafw/MaaFramework.dll",
            ]
        }
        report["passed"] = True
    except Exception as exc:
        report["passed"] = False
        report["error"] = str(exc)
        raise
    finally:
        save_json(args.checks / "BUILD_RESULT.json", report)

    for directory in ("config", "debug"):
        generated = install / directory
        if generated.exists():
            shutil.move(str(generated), str(args.checks / f"agent-{directory}"))
    save_json(install / "BUILD_SOURCE.json", report)
    (install / "BUILD_NATIVE_SOURCE.json").unlink()
    for pdb in install.rglob("*.pdb"):
        target = args.checks / "symbols" / pdb.relative_to(install)
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(pdb, target)
        pdb.unlink()
    (install / "LINGMOCHE.txt").write_text(
        f"MaaEnd-LingMoche {args.version}\nSource: {report['source_commit']}\n"
        f"Updates: {REPOSITORY}/releases\nRun MaaEnd.exe.\n"
        "Offline resource, Agent registration and navmesh checks passed. Game execution was not performed.\n",
        encoding="utf-8",
    )
    sums = [
        f"{sha256(path)}  {path.relative_to(install).as_posix()}"
        for path in sorted(install.rglob("*"))
        if path.is_file()
    ]
    (install / "SHA256SUMS.txt").write_text("\n".join(sums) + "\n", encoding="utf-8")
    archive = args.output / f"MaaEnd-LingMoche-win-x86_64-{args.version}.zip"
    with zipfile.ZipFile(
        archive, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=6
    ) as output:
        for path in sorted(install.rglob("*")):
            if path.is_file():
                output.write(path, path.relative_to(install).as_posix())
    (archive.with_suffix(".zip.sha256")).write_text(
        f"{sha256(archive)}  {archive.name}\n", encoding="utf-8"
    )
    print(f"Created {archive}")


if __name__ == "__main__":
    sys.stdout.reconfigure(encoding="utf-8")
    main()
