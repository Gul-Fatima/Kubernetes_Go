
#!/usr/bin/env python3

from pathlib import Path
import argparse
import numpy as np
import pandas as pd

ROOT = Path(__file__).resolve().parent
DATA = ROOT / "data"
PROCESSED = DATA / "processed" / "alibaba-gpu-v2026"

PILOT = PROCESSED / "pilot_workloads.parquet"

SERVER_FILE = (
    DATA / "raw" / "alibaba-gpu-v2026"
    / "server_hourly" / "day=0" / "hour=00"
    / "part-000.parquet"
)

POD_DIR = (
    DATA / "raw" / "alibaba-gpu-v2026"
    / "pod_hourly" / "day=0"
)

SUPPORTED_TYPES = {
    "training",
    "online_inference",
    "offline_inference",
}

DEFAULT_ARRIVAL_WINDOW_HOURS = 24
DEFAULT_SEED = 42


def load_cpu_statistics():
    """Calculate positive CPU request medians from pod_hourly."""

    files = sorted(POD_DIR.glob("hour=*/part-000.parquet"))

    if not files:
        raise FileNotFoundError(
            f"No pod_hourly partitions found in {POD_DIR}"
        )

    frames = []

    for file in files:
        print(f"Reading CPU statistics: {file}")

        part = pd.read_parquet(
            file,
            columns=[
                "job_type_public",
                "gpu_spec_public",
                "cpu_request_cores",
            ],
        )

        part["cpu_request_cores"] = pd.to_numeric(
            part["cpu_request_cores"],
            errors="coerce",
        )

        part = part[
            part["job_type_public"].isin(SUPPORTED_TYPES)
            & part["cpu_request_cores"].gt(0)
        ]

        frames.append(part)

    data = pd.concat(frames, ignore_index=True)

    if data.empty:
        raise ValueError("No positive CPU requests found in pod_hourly")

    group_medians = (
        data.groupby(
            ["job_type_public", "gpu_spec_public"]
        )["cpu_request_cores"]
        .median()
    )

    type_medians = (
        data.groupby("job_type_public")["cpu_request_cores"]
        .median()
    )

    global_median = float(
        data["cpu_request_cores"].median()
    )

    print(f"\nCPU reference observations: {len(data):,}")
    print(f"Global positive CPU median: {global_median}")

    return group_medians, type_medians, global_median


def prepare_workloads(max_workloads, arrival_window, seed):
    print(f"\nReading workloads: {PILOT}")

    df = pd.read_parquet(PILOT)

    required = {
        "pod_id",
        "job_type_public",
        "model_type_public",
        "gpu_spec_public",
        "gpu_request",
        "duration_hours",
    }

    missing = required - set(df.columns)

    if missing:
        raise ValueError(
            f"Missing workload columns: {sorted(missing)}"
        )

    # Keep supported AI workload categories.
    df = df[
        df["job_type_public"].isin(SUPPORTED_TYPES)
    ].copy()

    # Convert numeric fields.
    df["gpu_request"] = pd.to_numeric(
        df["gpu_request"], errors="coerce"
    )

    df["duration_hours"] = pd.to_numeric(
        df["duration_hours"], errors="coerce"
    )

    # Remove invalid records.
    df = df[
        df["pod_id"].notna()
        & df["duration_hours"].gt(0)
        & df["gpu_request"].ge(0)
    ].copy()

    # One workload per pod.
    df["pod_id"] = df["pod_id"].astype(str)

    df = (
        df.drop_duplicates("pod_id", keep="first")
        .reset_index(drop=True)
    )

    if df.empty:
        raise ValueError("No valid AI workloads remain after filtering")

    # CPU requirements are imputed from pod_hourly distributions.
    group_medians, type_medians, global_median = (
        load_cpu_statistics()
    )

    cpu_values = []

    for _, row in df.iterrows():
        key = (
            row["job_type_public"],
            row["gpu_spec_public"],
        )

        value = group_medians.get(key, np.nan)

        if pd.isna(value):
            value = type_medians.get(
                row["job_type_public"], np.nan
            )

        if pd.isna(value):
            value = global_median

        cpu_values.append(float(value))

    df["cpu_request_cores"] = cpu_values
    df["cpu_request_source"] = "imputed_from_pod_hourly"

    # Generate synthetic arrivals using a fixed random seed.
    rng = np.random.default_rng(seed)

    arrivals = rng.uniform(
        0,
        arrival_window * 3600,
        size=len(df),
    )

    df["arrival_time_seconds"] = np.sort(arrivals)

    # Preserve trace-provided execution duration.
    df["execution_time_seconds"] = (
        df["duration_hours"] * 3600
    )

    # Optional deterministic sample for development.
    if max_workloads > 0 and len(df) > max_workloads:
        df = (
            df.sample(
                n=max_workloads,
                random_state=seed,
            )
            .sort_values("arrival_time_seconds")
            .reset_index(drop=True)
        )

    # Prepare simulator input.
    output = pd.DataFrame({
        "id": df["pod_id"],
        "job_type": df["job_type_public"],
        "model_type": df["model_type_public"].fillna("").astype(str),
        "gpu_model": df["gpu_spec_public"],
        "cpu_request_cores": df["cpu_request_cores"],
        "memory_request_bytes": 0,
        "gpu_request": df["gpu_request"],
        "gpu_memory_request_bytes": 0,
        "arrival_time_seconds": df["arrival_time_seconds"],
        "execution_time_seconds": df["execution_time_seconds"],
        "source_server_id": (
            df["source_server_id"].fillna("").astype(str)
            if "source_server_id" in df.columns
            else ""
        ),
        "cpu_request_source": df["cpu_request_source"],
        "memory_request_source": "unavailable",
        "gpu_request_source": "trace",
        "gpu_memory_source": "unavailable_excluded",
        "arrival_time_source": "synthetic_uniform",
        "execution_time_source": "trace_duration",
    })

    return output


def prepare_servers():
    print(f"\nReading server inventory: {SERVER_FILE}")

    df = pd.read_parquet(SERVER_FILE)

    required = {
        "server_id",
        "cluster_id",
        "gpu_spec_public",
        "gpu_count",
        "cpu_capacity_cores",
    }

    missing = required - set(df.columns)

    if missing:
        raise ValueError(
            f"Missing server columns: {sorted(missing)}"
        )

    df = df.dropna(
        subset=[
            "server_id",
            "gpu_spec_public",
            "gpu_count",
            "cpu_capacity_cores",
        ]
    ).copy()

    df = df.drop_duplicates(
        subset=["server_id"],
        keep="first",
    )

    df["gpu_count"] = pd.to_numeric(
        df["gpu_count"], errors="coerce"
    )

    df["cpu_capacity_cores"] = pd.to_numeric(
        df["cpu_capacity_cores"], errors="coerce"
    )

    df = df[
        df["gpu_count"].ge(0)
        & df["cpu_capacity_cores"].gt(0)
    ].copy()

    output = pd.DataFrame({
        "id": df["server_id"].astype(str),
        "cluster_id": df["cluster_id"].fillna("default").astype(str),
        "cpu_cores": df["cpu_capacity_cores"],
        "memory_bytes": 0,
        "gpu_count": df["gpu_count"],
        "gpu_model": df["gpu_spec_public"].astype(str),
        "gpu_memory_bytes": 0,
        "hardware_source": "alibaba_server_hour00",
    })

    return output


def main():
    parser = argparse.ArgumentParser(
        description="Prepare Alibaba GPU workload and server data for EAWSK."
    )

    parser.add_argument(
        "--max-workloads",
        type=int,
        default=0,
        help="Maximum workloads to export; 0 exports all.",
    )

    parser.add_argument(
        "--arrival-window-hours",
        type=float,
        default=DEFAULT_ARRIVAL_WINDOW_HOURS,
    )

    parser.add_argument(
        "--seed",
        type=int,
        default=DEFAULT_SEED,
    )

    args = parser.parse_args()

    if args.max_workloads < 0:
        parser.error("--max-workloads cannot be negative")

    if args.arrival_window_hours <= 0:
        parser.error("--arrival-window-hours must be positive")

    if not PILOT.exists():
        raise FileNotFoundError(PILOT)

    if not SERVER_FILE.exists():
        raise FileNotFoundError(SERVER_FILE)

    PROCESSED.mkdir(parents=True, exist_ok=True)

    workloads = prepare_workloads(
        max_workloads=args.max_workloads,
        arrival_window=args.arrival_window_hours,
        seed=args.seed,
    )

    servers = prepare_servers()

    workload_output = PROCESSED / "go_workloads.csv"
    server_output = PROCESSED / "go_servers.csv"

    workloads.to_csv(workload_output, index=False)
    servers.to_csv(server_output, index=False)

    print("\n" + "=" * 45)
    print("PREPARATION COMPLETE")
    print("=" * 45)

    print(f"Workloads: {len(workloads):,}")
    print(f"Servers:   {len(servers):,}")

    print("\nWorkload distribution:")
    print(workloads["job_type"].value_counts().to_string())

    print(
        "\nCPU requirement source:",
        workloads["cpu_request_source"].value_counts().to_dict(),
    )

    print(f"\nArrival window: {args.arrival_window_hours} hours")
    print(f"Random seed: {args.seed}")

    print(f"\nWorkload CSV: {workload_output}")
    print(f"Server CSV:   {server_output}")

    print("\nNotes:")
    print("- CPU requirements are statistically imputed.")
    print("- GPU requests are retained from the workload trace.")
    print("- Host RAM and GPU memory are excluded as constraints.")
    print("- Arrival times are synthetic.")
    print("- Execution durations are trace-provided.")


if __name__ == "__main__":
    main()
