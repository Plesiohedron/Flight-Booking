FROM python:3.12-slim
WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir \
    --trusted-host pypi.org \
    --trusted-host pypi.python.org \
    --trusted-host files.pythonhosted.org \
    -r requirements.txt

# Generate Python gRPC stubs (grpcio-tools bundles google/protobuf/*.proto)
COPY proto/ proto/
RUN mkdir -p app/gen/flight/v1 && \
    python -m grpc_tools.protoc \
      -I proto \
      --python_out=app/gen \
      --grpc_python_out=app/gen \
      proto/flight/v1/flight.proto && \
    touch app/gen/__init__.py app/gen/flight/__init__.py app/gen/flight/v1/__init__.py && \
    # grpc_tools генерирует "from flight.v1 import ..." — патчим на абсолютный путь
    sed -i 's/from flight\.v1 import/from app.gen.flight.v1 import/g' \
      app/gen/flight/v1/flight_pb2_grpc.py

COPY . .
EXPOSE 8000
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]

