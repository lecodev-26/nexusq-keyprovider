#!/usr/bin/env python3
"""Generate an SPDX 2.3 inventory of direct CI actions and declared tool inputs."""
from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
import platform
import re
import subprocess
from pathlib import Path


def command_output(*args: str) -> str:
    try:
        result = subprocess.run(args, check=True, text=True, capture_output=True)
        return (result.stdout or result.stderr).strip()
    except (OSError, subprocess.CalledProcessError):
        return 'unavailable'


def command(*args: str) -> str:
    return command_output(*args).splitlines()[0]


def os_release() -> str:
    path = Path('/etc/os-release')
    if path.exists():
        values = {}
        for line in path.read_text(encoding='utf-8').splitlines():
            if '=' in line:
                key, value = line.split('=', 1)
                values[key] = value.strip().strip('"')
        return f"{values.get('PRETTY_NAME', 'unknown')} ({values.get('VERSION_ID', 'unknown')})"
    return platform.platform()


def workflow_actions(workflow_text: str) -> list[tuple[str, str, str]]:
    actions: list[tuple[str, str, str]] = []
    for line in workflow_text.splitlines():
        use = re.search(r'\buses:\s*([^\s#]+)', line)
        if not use:
            continue
        ref = use.group(1)
        if ref.startswith('./') or '@' not in ref:
            continue
        repo, version = ref.rsplit('@', 1)
        url = repo if repo.startswith('docker://') else f'https://github.com/{repo}'
        actions.append((repo, version, url))
    return sorted(set(actions))


def package(name: str, version: str, download: str, purpose: str = 'APPLICATION') -> dict:
    safe = re.sub(r'[^A-Za-z0-9.-]+', '-', name).strip('-') or 'package'
    suffix = hashlib.sha256(f'{name}@{version}'.encode()).hexdigest()[:10]
    return {
        'name': name,
        'SPDXID': f'SPDXRef-{safe[:48]}-{suffix}',
        'versionInfo': version,
        'downloadLocation': download or 'NOASSERTION',
        'filesAnalyzed': False,
        'licenseConcluded': 'NOASSERTION',
        'licenseDeclared': 'NOASSERTION',
        'copyrightText': 'NOASSERTION',
        'primaryPackagePurpose': purpose,
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument('--workflow', default='.github/workflows/opentofu.yml')
    parser.add_argument('--output', required=True)
    parser.add_argument('--manifest', required=True)
    parser.add_argument('--syft-version', default='')
    args = parser.parse_args()

    adapter_dir = Path(__file__).resolve().parents[1]
    workflow_text = Path(args.workflow).read_text(encoding='utf-8')
    actions = workflow_actions(workflow_text)
    packages = [package(name, version, url) for name, version, url in actions]

    runner_labels = sorted(set(re.findall(r'^\s*(?:runs-on:|runner:|-\s*runner:)\s*([A-Za-z0-9_.-]+)', workflow_text, re.MULTILINE)))
    packages.append(package('GitHub Actions runner labels', ', '.join(runner_labels), 'https://github.com/actions/runner-images'))

    tofu_versions = sorted(set(re.findall(r"tofu_version:\s*['\"]?([^'\"#\s]+)", workflow_text)))
    for version in tofu_versions:
        packages.append(package('OpenTofu CLI', version, 'https://github.com/opentofu/opentofu'))

    go_matrix = re.search(r"go:\s*\['([^']+)',\s*'([^']+)'\]", workflow_text)
    if go_matrix:
        packages.append(package('Go test matrix', f'{go_matrix.group(1)}; {go_matrix.group(2)} (latest patch resolved at runtime)', 'https://go.dev/dl/'))

    nexusq_ref = re.search(r'repository:\s*lecodev-26/nexus-q\s*\n\s*ref:\s*([0-9a-f]{40})', workflow_text)
    if nexusq_ref:
        packages.append(package('NEXUS-Q source', nexusq_ref.group(1), 'https://github.com/lecodev-26/nexus-q'))

    if any(name == 'dtolnay/rust-toolchain' for name, _, _ in actions):
        packages.append(package('Rust toolchain selector', 'stable (resolved at CI runtime; action ref is pinned)', 'https://www.rust-lang.org/tools/install'))

    s3mock_path = adapter_dir / 'tests/integration/s3mock-e2e.sh'
    s3mock_match = re.search(r'S3MOCK_IMAGE="([^"]+)"', s3mock_path.read_text(encoding='utf-8'))
    if s3mock_match:
        packages.append(package('S3Mock container image', s3mock_match.group(1), 'https://hub.docker.com/r/adobe/s3mock'))

    go_version = command('go', 'version')
    git_version = command('git', '--version')
    bash_version = command('bash', '--version')
    python_version = command('python3', '--version')
    tar_version = command('tar', '--version')
    checksum_version = command('sha256sum', '--version')
    syft_version = args.syft_version or 'not used in this job'
    runner_label = 'ubuntu-24.04'
    runner_image = f"{os.environ.get('ImageOS', 'unknown')} {os.environ.get('ImageVersion', 'unknown')}"

    tools = [
        ('Go toolchain (release build)', go_version, 'https://go.dev/dl/'),
        ('Git (release build runner)', git_version, 'https://git-scm.com/'),
        ('Bash (release build runner)', bash_version, 'https://www.gnu.org/software/bash/'),
        ('Python (release build runner)', python_version, 'https://www.python.org/'),
        ('tar (release build runner)', tar_version, 'https://www.gnu.org/software/tar/'),
        ('sha256sum / GNU coreutils (release build runner)', checksum_version, 'https://www.gnu.org/software/coreutils/'),
        ('Syft', syft_version, 'https://github.com/anchore/syft'),
        ('GitHub-hosted release-build runner image', runner_image, 'https://github.com/actions/runner-images'),
    ]
    packages.extend(package(name, version, url, 'OPERATING-SYSTEM' if 'runner image' in name else 'APPLICATION') for name, version, url in tools)

    created = dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat().replace('+00:00', 'Z')
    digest_input = json.dumps(packages, sort_keys=True).encode()
    namespace_id = hashlib.sha256(digest_input + created.encode()).hexdigest()
    package_ids = [item['SPDXID'] for item in packages]
    document = {
        'spdxVersion': 'SPDX-2.3',
        'dataLicense': 'CC0-1.0',
        'SPDXID': 'SPDXRef-DOCUMENT',
        'name': 'NEXUS-Q OpenTofu adapter CI environment',
        'documentNamespace': f'https://spdx.org/spdxdocs/nexusq-opentofu-ci-environment-{namespace_id}',
        'creationInfo': {
            'creators': ['Tool: nexusq-keyprovider-opentofu-ci-sbom-generator'],
            'created': created,
            'comment': 'Direct actions declared across the workflow; declared OpenTofu integration inputs; and directly invoked tools plus actual runner image for the release-build job.',
        },
        'packages': packages,
        'documentDescribes': package_ids,
        'relationships': [
            {'spdxElementId': 'SPDXRef-DOCUMENT', 'relationshipType': 'DESCRIBES', 'relatedSpdxElement': package_id}
            for package_id in package_ids
        ],
    }
    Path(args.output).parent.mkdir(parents=True, exist_ok=True)
    Path(args.output).write_text(json.dumps(document, indent=2) + '\n', encoding='utf-8')

    module_graph = subprocess.run(
        ['go', 'list', '-m', 'all'], cwd=adapter_dir, check=True, text=True, capture_output=True
    ).stdout.strip().replace('\n', ', ')
    manifest = [
        'NEXUS-Q OpenTofu adapter CI environment inventory',
        f'Release-build runner label: {runner_label}',
        f'Release-build runner image: {runner_image}',
        f'Release-build OS: {os_release()}',
        f'Release-build kernel/architecture: {platform.platform()} / {platform.machine()}',
        f'Release-build Go: {go_version}',
        f'Release-build Go environment: {command_output("go", "env", "GOOS", "GOARCH", "CGO_ENABLED", "GOTOOLCHAIN", "GOWORK", "GOFLAGS").replace(chr(10), ", ")}',
        f'Release-build Go module graph: {module_graph}',
        f'Release-build Git: {git_version}',
        f'Release-build Bash: {bash_version}',
        f'Release-build Python: {python_version}',
        f'Release-build tar: {tar_version}',
        f'Release-build sha256sum: {checksum_version}',
        f'Syft: {syft_version}',
        'Direct actions declared across CI workflow:',
    ]
    manifest.extend(f'  - {name}@{version}' for name, version, _ in actions)
    manifest.extend([
        f'Runner labels declared in workflow: {", ".join(runner_labels)}',
        f'OpenTofu versions declared in workflow: {", ".join(tofu_versions)}',
    ])
    if go_matrix:
        manifest.append(f'Go test matrix: {go_matrix.group(1)}, {go_matrix.group(2)} (latest patch resolved at runtime)')
    if nexusq_ref:
        manifest.append(f'NEXUS-Q source commit: {nexusq_ref.group(1)}')
    if s3mock_match:
        manifest.append(f'S3Mock image: {s3mock_match.group(1)}')
    manifest.extend([
        'Rust toolchain: stable selected by the pinned dtolnay/rust-toolchain action; exact patch resolved at runtime.',
        '',
        'Scope: direct workflow actions, declared integration tool/source versions, and directly invoked release-build tools plus the actual release-build runner image.',
        'Excludes: all preinstalled packages on every runner, action-internal transitive dependencies, exact Rust stable resolution, GitHub service internals, and external NEXUS-Q deployment software.',
    ])
    Path(args.manifest).parent.mkdir(parents=True, exist_ok=True)
    Path(args.manifest).write_text('\n'.join(manifest) + '\n', encoding='utf-8')


if __name__ == '__main__':
    main()
