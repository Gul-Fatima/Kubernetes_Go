
from typing import Any


def score_nodes(nodes: list[dict[str, Any]]) -> list[dict[str, Any]]:
    """
    Rank feasible nodes by estimated incremental energy.

    Each node must contain:
      name
      current_gpu_utilization: 0.0 to 1.0
      projected_gpu_utilization: 0.0 to 1.0
      current_cpu_utilization: 0.0 to 1.0
      projected_cpu_utilization: 0.0 to 1.0

    Power model is illustrative, not measured hardware power.
    """
    scored = []

    idle_power_w = 1000.0
    dynamic_power_w = 1700.0
    duration_hours = 1.0

    for node in nodes:
        current_gpu = float(node["current_gpu_utilization"])
        current_cpu = float(node["current_cpu_utilization"])
        projected_gpu = float(node["projected_gpu_utilization"])
        projected_cpu = float(node["projected_cpu_utilization"])

        values = [current_gpu, current_cpu, projected_gpu, projected_cpu]
        if any(not 0.0 <= value <= 1.0 for value in values):
            raise ValueError(f"Utilization must be between 0 and 1: {node['name']}")

        current_load = (current_gpu + current_cpu) / 2
        projected_load = (projected_gpu + projected_cpu) / 2

        current_power = idle_power_w + dynamic_power_w * current_load
        projected_power = idle_power_w + dynamic_power_w * projected_load

        incremental_energy_wh = max(
            0.0,
            projected_power - current_power,
        ) * duration_hours

        scored.append({
            "name": node["name"],
            "estimated_incremental_energy_wh": incremental_energy_wh,
        })

    # Lower energy is better. Node name provides deterministic tie-breaking.
    scored.sort(key=lambda item: (
        item["estimated_incremental_energy_wh"],
        item["name"],
    ))

    for rank, item in enumerate(scored):
        item["score"] = 100 - round(
            100 * rank / max(len(scored) - 1, 1)
        )

    return scored
