import os
import sys
import time
import logging
import asyncio
from concurrent import futures
import grpc

# Bổ sung thư mục proto vào sys.path để Python import sạch sẽ
sys.path.append(os.path.join(os.path.dirname(__file__), "proto"))

from proto import tts_pb2, tts_pb2_grpc
from engine import synthesize_standard_sync, synthesize_fast_async

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("GrpcTTSServer")

class TTSServiceServicer(tts_pb2_grpc.TTSServiceServicer):
    """Implementation của TTSService gRPC Server cho Python Core-TTS Engine"""

    def SynthesizeStandard(self, request, context):
        logger.info(f"[gRPC] SynthesizeStandard: voice='{request.voice}', speed={request.speed}, text_len={len(request.text)}")
        try:
            audio_bytes = synthesize_standard_sync(request.text, request.voice, request.speed)
            yield tts_pb2.TTSChunkResponse(
                chunk_index=1,
                total_chunks=1,
                audio_bytes=audio_bytes,
                status="done",
                error_msg=""
            )
        except Exception as e:
            logger.error(f"[gRPC] SynthesizeStandard error: {e}")
            yield tts_pb2.TTSChunkResponse(
                chunk_index=1,
                total_chunks=1,
                audio_bytes=b"",
                status="error",
                error_msg=str(e)
            )

    def SynthesizeFast(self, request, context):
        logger.info(f"[gRPC] SynthesizeFast: voice='{request.voice}', speed={request.speed}, text_len={len(request.text)}")
        try:
            audio_bytes = asyncio.run(synthesize_fast_async(request.text, request.voice, request.speed))
            yield tts_pb2.TTSChunkResponse(
                chunk_index=1,
                total_chunks=1,
                audio_bytes=audio_bytes,
                status="done",
                error_msg=""
            )
        except Exception as e:
            logger.error(f"[gRPC] SynthesizeFast error: {e}")
            yield tts_pb2.TTSChunkResponse(
                chunk_index=1,
                total_chunks=1,
                audio_bytes=b"",
                status="error",
                error_msg=str(e)
            )

def serve_grpc(port=50051):
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    tts_pb2_grpc.add_TTSServiceServicer_to_server(TTSServiceServicer(), server)
    server.add_insecure_port(f"[::]:{port}")
    server.start()
    logger.info(f"🚀 gRPC Core-TTS High-Performance Server running on port {port}")
    return server

if __name__ == "__main__":
    server = serve_grpc()
    try:
        server.wait_for_termination()
    except KeyboardInterrupt:
        server.stop(0)
