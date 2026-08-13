"""
gRPC server for the Example TTS Engine.

Mirrors the HTTP endpoints: GetInfo, GetVoices, Synthesize, CloneVoice.
Uses dummy audio generation from `audio_utils.py`.
"""

import os
import sys
import logging
from concurrent import futures
import grpc

sys.path.append(os.path.join(os.path.dirname(__file__), "proto"))

from utils.audio_utils import generate_dummy_wav
from schemas import UniversalManifest
from proto import tts_pb2, tts_pb2_grpc

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("GrpcExampleTTSServer")

MAX_MESSAGE_BYTES = 50 * 1024 * 1024  # 50 MB
EXAMPLE_DELAY_SEC = float(os.getenv("EXAMPLE_DELAY_SEC", "0.0"))

# Giọng đọc theo từng Model / Mode — cùng danh sách với main.py
MODEL_VOICES = {
    "fast": [
        {"id": "hoai_my", "name": "Hoài Mỹ", "metadata": {"gender": "Nữ", "region": "Miền Bắc", "style": "Tự nhiên"}},
        {"id": "nam_minh", "name": "Nam Minh", "metadata": {"gender": "Nam", "region": "Miền Nam", "style": "Bản tin"}},
    ],
    "standard": [
        {"id": "hoai_my", "name": "Hoài Mỹ", "metadata": {"gender": "Nữ", "region": "Miền Bắc", "style": "Tự nhiên"}},
        {"id": "nam_minh", "name": "Nam Minh", "metadata": {"gender": "Nam", "region": "Miền Nam", "style": "Bản tin"}},
        {"id": "thu_hien", "name": "Thu Hiền", "metadata": {"gender": "Nữ", "region": "Miền Trung", "style": "Dịu dàng"}},
    ],
}


class TTSServiceServicer(tts_pb2_grpc.TTSServiceServicer):
    """gRPC server triển khai contract TTSService cho Example TTS Engine."""

    def GetInfo(self, request, context):
        """Trả manifest JSON — nền tảng đọc để biết engine hỗ trợ những UI slider / emotion / mode gì."""
        manifest = UniversalManifest()
        manifest_json = manifest.model_dump_json()
        logger.info("[gRPC] GetInfo: returning manifest (%d bytes)", len(manifest_json))
        return tts_pb2.GetInfoResponse(manifest_json=manifest_json)

    def GetVoices(self, request, context):
        """Trả danh sách giọng preset cho model_id được yêu cầu."""
        model_id = request.model_id or None
        if not model_id or model_id == "all":
            seen = set()
            voices = []
            for v_list in MODEL_VOICES.values():
                for v in v_list:
                    if v["id"] not in seen:
                        seen.add(v["id"])
                        voices.append(v)
        else:
            voices = MODEL_VOICES.get(model_id, [])

        logger.info("[gRPC] GetVoices: model_id=%s, count=%d", model_id, len(voices))
        return tts_pb2.GetVoicesResponse(
            voices=[
                tts_pb2.VoiceInfo(
                    id=v["id"],
                    name=v["name"],
                    descriptions=v["descriptions"],
                )
                for v in voices
            ]
        )

    def Synthesize(self, request, context):
        """Sinh âm thanh — nhận voice_id, speed, text... và trả raw WAV bytes."""
        engine = request.engine or "standard"
        logger.info(
            "[gRPC] Synthesize: engine=%s, voice_id=%s, speed=%.1f, text_len=%d",
            engine, request.voice_id, request.speed, len(request.text),
        )

        if not request.text.strip():
            return tts_pb2.SynthesizeResponse(error_msg="Empty text input")

        try:
            import random
            processing_time = random.uniform(2.0, 5.0)
            time.sleep(processing_time)

            duration = random.uniform(30.0, 60.0)
            wav_bytes = generate_dummy_wav(duration)
            return tts_pb2.SynthesizeResponse(audio_bytes=wav_bytes)
        except Exception as e:
            logger.error("[gRPC] Synthesize error: %s", e)
            return tts_pb2.SynthesizeResponse(error_msg=str(e))

    def CloneVoice(self, request, context):
        """Nhận file âm thanh tham chiếu và trả voice_id mới."""
        import uuid
        clone_id = f"clone_example_{uuid.uuid4().hex[:8]}"
        logger.info(
            "[gRPC] CloneVoice: name=%s, engine=%s, audio_len=%d",
            request.name, request.engine, len(request.audio_bytes),
        )
        return tts_pb2.CloneVoiceResponse(
            voice_id=clone_id,
            name=request.name,
            status="success",
        )


def serve_grpc(port=50051):
    """Khởi động gRPC server ở background thread."""
    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=10),
        options=[
            ("grpc.max_receive_message_length", MAX_MESSAGE_BYTES),
            ("grpc.max_send_message_length", MAX_MESSAGE_BYTES),
        ],
    )
    tts_pb2_grpc.add_TTSServiceServicer_to_server(TTSServiceServicer(), server)
    server.add_insecure_port(f"0.0.0.0:{port}")
    server.start()
    logger.info("gRPC Example TTS Server running on port %d", port)
    return server


if __name__ == "__main__":
    server = serve_grpc()
    try:
        server.wait_for_termination()
    except KeyboardInterrupt:
        server.stop(0)
