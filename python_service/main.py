
from typing import Any

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel

from policies.energy_aware import score_nodes

app = FastAPI(title="EAWSK Kubernetes Scheduler Extender")


class ExtenderArgs(BaseModel):
    pod: dict[str, Any]
    nodes: dict[str, Any]


@app.get("/health")
def health():
    return {"status": "ok"}


@app.post("/filter")
def filter_nodes(request: ExtenderArgs):
    # Kubernetes has already applied its standard scheduling filters.
    # Preserve those candidate nodes; add custom constraints here later.
    return {
        "nodes": request.nodes,
        "failedNodes": {},
        "failedAndUnresolvable": {},
        "error": "",
    }


@app.post("/prioritize")
def prioritize_nodes(request: ExtenderArgs):
    nodes = request.nodes.get("items", [])

    # These annotations must come from a real telemetry source or
    # be deliberately set for testing. Missing telemetry must not
    # be presented as an energy measurement.
    annotation_keys = {
        "current_gpu_utilization": "eawsk.io/current-gpu-utilization",
        "current_cpu_utilization": "eawsk.io/current-cpu-utilization",
        "projected_gpu_utilization": "eawsk.io/projected-gpu-utilization",
        "projected_cpu_utilization": "eawsk.io/projected-cpu-utilization",
    }

    # Until every candidate has telemetry, give all nodes a neutral
    # score and let the scheduler's other scoring plugins contribute.
    if not nodes:
        return []

    inputs = []
    for node in nodes:
        metadata = node.get("metadata", {})
        annotations = metadata.get("annotations") or {}

        if not all(key in annotations for key in annotation_keys.values()):
            return [
                {"host": item.get("metadata", {}).get("name", ""), "score": 5}
                for item in nodes
            ]

        item = {"name": metadata.get("name", "")}
        for field, annotation in annotation_keys.items():
            item[field] = annotations[annotation]

        inputs.append(item)

    try:
        ranked = score_nodes(inputs)
    except (KeyError, TypeError, ValueError) as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc

    # Kubernetes extender priorities use the range 0–10.
    return [
        {
            "host": item["name"],
            "score": round(item["score"] / 10),
        }
        for item in ranked
    ]
