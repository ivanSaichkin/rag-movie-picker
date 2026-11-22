import argparse

import grpc
from google.protobuf.json_format import MessageToJson

from rag.v1 import rag_pb2, rag_pb2_grpc


def main() -> None:
    parser = argparse.ArgumentParser(description="Query the local movie RAG service")
    parser.add_argument("query")
    parser.add_argument("--limit", type=int, default=5)
    parser.add_argument("--address", default="127.0.0.1:50051")
    parser.add_argument("--strategy", choices=["bm25", "transformer", "hybrid"])
    parser.add_argument("--candidate-limit", type=int, help="N candidates before final selection; must be >= limit")
    parser.add_argument("--generate-explanations", action=argparse.BooleanOptionalAction, default=None)
    args = parser.parse_args()
    request = rag_pb2.RecommendRequest(query=args.query, limit=args.limit)
    if args.candidate_limit is not None:
        request.candidate_limit = args.candidate_limit
    if args.strategy is not None:
        request.strategy = args.strategy
    if args.generate_explanations is not None:
        request.generate_explanations = args.generate_explanations
    with grpc.insecure_channel(args.address) as channel:
        response = rag_pb2_grpc.RagServiceStub(channel).Recommend(
            request,
            timeout=60,
        )
        print(MessageToJson(response, preserving_proto_field_name=True, ensure_ascii=False))


if __name__ == "__main__":
    main()
