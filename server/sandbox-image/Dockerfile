FROM python:3.12-slim

COPY quantsim_sdk/ /sdk/
RUN pip install --no-cache-dir /sdk/

RUN adduser --disabled-password --no-create-home --uid 1001 sandbox
USER sandbox

WORKDIR /script

ENTRYPOINT ["python3", "-u", "/script/trader.py"]
