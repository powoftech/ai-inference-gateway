from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class GenerateRequest(_message.Message):
    __slots__ = ("request_id", "model", "messages", "tenant_id")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    MODEL_FIELD_NUMBER: _ClassVar[int]
    MESSAGES_FIELD_NUMBER: _ClassVar[int]
    TENANT_ID_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    model: str
    messages: _containers.RepeatedCompositeFieldContainer[Message]
    tenant_id: str
    def __init__(self, request_id: _Optional[str] = ..., model: _Optional[str] = ..., messages: _Optional[_Iterable[_Union[Message, _Mapping]]] = ..., tenant_id: _Optional[str] = ...) -> None: ...

class Message(_message.Message):
    __slots__ = ("role", "content")
    ROLE_FIELD_NUMBER: _ClassVar[int]
    CONTENT_FIELD_NUMBER: _ClassVar[int]
    role: str
    content: str
    def __init__(self, role: _Optional[str] = ..., content: _Optional[str] = ...) -> None: ...

class GenerateResponse(_message.Message):
    __slots__ = ("request_id", "token", "is_finished", "tokens_generated", "processing_time_ms")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    TOKEN_FIELD_NUMBER: _ClassVar[int]
    IS_FINISHED_FIELD_NUMBER: _ClassVar[int]
    TOKENS_GENERATED_FIELD_NUMBER: _ClassVar[int]
    PROCESSING_TIME_MS_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    token: str
    is_finished: bool
    tokens_generated: int
    processing_time_ms: int
    def __init__(self, request_id: _Optional[str] = ..., token: _Optional[str] = ..., is_finished: bool = ..., tokens_generated: _Optional[int] = ..., processing_time_ms: _Optional[int] = ...) -> None: ...
