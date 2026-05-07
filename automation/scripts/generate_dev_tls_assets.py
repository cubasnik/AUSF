from __future__ import annotations

import argparse
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
OUTPUT_DIR = ROOT / "automation" / ".tls-dev"
CA_CERT = OUTPUT_DIR / "dev-root-ca.crt"
CA_KEY = OUTPUT_DIR / "dev-root-ca.key"
GO_CERT = OUTPUT_DIR / "ausf-go.crt"
GO_KEY = OUTPUT_DIR / "ausf-go.key"
CONTROL_PLANE_CERT = OUTPUT_DIR / "ausf-control-plane.crt"
CONTROL_PLANE_KEY = OUTPUT_DIR / "ausf-control-plane.key"
CONTROL_PLANE_P12 = OUTPUT_DIR / "ausf-control-plane.p12"
P12_PASSWORD = "changeit"


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Generate local dev TLS assets for the AUSF TLS smoke suite.")
    parser.add_argument("--force", action="store_true", help="Regenerate assets even if they already exist.")
    return parser.parse_args()


def run_command(*arguments: str) -> None:
    print(f"==> {' '.join(arguments)}", flush=True)
    subprocess.run(arguments, cwd=ROOT, check=True)


def write_extfile(path: Path, common_name: str) -> None:
    path.write_text(
        "\n".join(
            [
                "[v3_req]",
                "basicConstraints=CA:FALSE",
                "keyUsage=digitalSignature,keyEncipherment",
                "extendedKeyUsage=serverAuth",
                "subjectAltName=@alt_names",
                "",
                "[alt_names]",
                f"DNS.1={common_name}",
                "DNS.2=localhost",
                "IP.1=127.0.0.1",
            ]
        ) + "\n",
        encoding="utf-8",
    )


def ensure_dev_tls_assets(force: bool = False) -> None:
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    targets = [CA_CERT, CA_KEY, GO_CERT, GO_KEY, CONTROL_PLANE_CERT, CONTROL_PLANE_KEY, CONTROL_PLANE_P12]
    if not force and all(path.exists() for path in targets):
        print(f"TLS assets already present in {OUTPUT_DIR}", flush=True)
        return

    for path in OUTPUT_DIR.iterdir():
        if path.is_file():
            path.unlink()

    go_extfile = OUTPUT_DIR / "ausf-go.ext"
    control_plane_extfile = OUTPUT_DIR / "ausf-control-plane.ext"
    write_extfile(go_extfile, "ausf-go")
    write_extfile(control_plane_extfile, "ausf-control-plane")

    run_command(
        "openssl", "req", "-x509", "-newkey", "rsa:2048", "-sha256", "-nodes",
        "-keyout", str(CA_KEY),
        "-out", str(CA_CERT),
        "-days", "3650",
        "-subj", "/CN=AUSF Dev Root CA",
    )

    generate_leaf_certificate("ausf-go", GO_KEY, GO_CERT, go_extfile)
    generate_leaf_certificate("ausf-control-plane", CONTROL_PLANE_KEY, CONTROL_PLANE_CERT, control_plane_extfile)

    run_command(
        "openssl", "pkcs12", "-export",
        "-inkey", str(CONTROL_PLANE_KEY),
        "-in", str(CONTROL_PLANE_CERT),
        "-certfile", str(CA_CERT),
        "-out", str(CONTROL_PLANE_P12),
        "-passout", f"pass:{P12_PASSWORD}",
        "-name", "ausf-control-plane",
    )

    print(f"Generated dev TLS assets in {OUTPUT_DIR}", flush=True)


def generate_leaf_certificate(common_name: str, key_path: Path, cert_path: Path, extfile: Path) -> None:
    csr_path = cert_path.with_suffix(".csr")
    run_command(
        "openssl", "req", "-newkey", "rsa:2048", "-sha256", "-nodes",
        "-keyout", str(key_path),
        "-out", str(csr_path),
        "-subj", f"/CN={common_name}",
    )
    run_command(
        "openssl", "x509", "-req",
        "-in", str(csr_path),
        "-CA", str(CA_CERT),
        "-CAkey", str(CA_KEY),
        "-CAcreateserial",
        "-out", str(cert_path),
        "-days", "3650",
        "-sha256",
        "-extfile", str(extfile),
        "-extensions", "v3_req",
    )
    csr_path.unlink(missing_ok=True)


if __name__ == "__main__":
    args = parse_args()
    ensure_dev_tls_assets(force=args.force)
