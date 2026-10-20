import argparse

import grpc
from google.protobuf.json_format import MessageToJson

from rag.v1 import rag_pb2, rag_pb2_grpc


def main() -> None:
    parser = argparse.ArgumentParser(description="Query the local movie RAG service")
    parser.add_argument("query")
    parser.add_argument("--limit", type=int, default=5)
    parser.add_argument("--address", default="127.0.0.1:50051")
    args = parser.parse_args()
    with grpc.insecure_channel(args.address) as channel:
        response = rag_pb2_grpc.RagServiceStub(channel).Recommend(
            rag_pb2.RecommendRequest(query=args.query, limit=args.limit),
            timeout=60,
        )
        print(MessageToJson(response, preserving_proto_field_name=True, ensure_ascii=False))


if __name__ == "__main__":
    main()
