from typing import Any, Protocol, TypedDict


class PodRequest(TypedDict, total=False):
    pod_id: str
    workload_id: str | None
    job_type: str
    cpu_request_cores: float
    gpu_request: float
    gpu_spec: str
    duration_hours: float


class CandidateServer(TypedDict, total=False):
    server_id: str
    gpu_spec: str
    gpu_capacity: float
    available_gpu: float
    cpu_capacity_cores: float
    available_cpu_cores: float
    current_gpu_utilization: float
    current_cpu_utilization: float
    projected_gpu_utilization: float
    projected_cpu_utilization: float


class RankedServer(TypedDict, total=False):
    server_id: str
    score: float
    reason: str
    metrics: dict[str, Any]


class SchedulingPolicy(Protocol):
    def rank(
        self,
        pod: PodRequest,
        candidates: list[CandidateServer],
    ) -> list[RankedServer]:
        ...
