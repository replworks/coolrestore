# coolrestore 구현 스펙 (수정본)

Go로 독립 실행형 인프라 CLI 도구를 구현하라.

## 프로젝트명

coolrestore

## 목적

Coolify가 S3-compatible storage에 생성한 Persistent Storage backup archive를 다운로드하여 지정된 서버 디렉터리에 안전하게 복원한다.

## 중요한 설계 원칙

- Laravel에 의존하지 않는다.
- PHP에 의존하지 않는다.
- Coolify API에 의존하지 않는다.
- 데이터베이스를 복원하지 않는다.
- 애플리케이션 비즈니스 로직을 검사하지 않는다.
- 애플리케이션 중지·재시작을 수행하지 않는다.
- S3 archive 다운로드와 파일 복원만 담당한다.
- 여러 Laravel, Go, Node.js 서비스에서 사용할 수 있는 범용 CLI로 만든다.

## 기술 요구사항

- Go 공식 최신 안정 버전을 사용한다.
- 구현 시작 전에 https://go.dev/dl/ 에서 최신 안정 버전을 확인한다.
- 현재 기준 Go 1.27.1 이상을 사용한다.
- go.mod와 go toolchain 설정에도 실제 사용 버전을 명시한다.
- **GitHub Actions의 `setup-go`가 해당 버전을 아직 지원하지 않을 경우를 대비해, workflow에 `go install golang.org/dl/go1.27.1@latest` 방식의 폴백 절차를 README와 workflow 주석에 명시한다.**
- 표준 라이브러리를 우선 사용한다.
- S3 접근은 AWS SDK for Go v2를 사용한다.
- `archive/tar`, `compress/gzip`을 사용한다.
- Linux `arm64`와 `amd64`를 모두 빌드한다.
- Coolify 서버가 aarch64이므로 `linux/arm64`를 필수 대상으로 한다.
- cgo 없이 정적 바이너리로 빌드한다.

## 명령 이름

coolrestore

## 기본 사용법

```
coolrestore \
  --source s3://bucket/path/storage-backup.tar.gz \
  --target /data/wifinote/storage \
  --dry-run
```

## 실제 복원

```
coolrestore \
  --source s3://bucket/path/storage-backup.tar.gz \
  --target /data/wifinote/storage \
  --confirm
```

## S3 설정

```
AWS_ACCESS_KEY_ID
AWS_SECRET_ACCESS_KEY
AWS_REGION
AWS_ENDPOINT_URL
AWS_S3_FORCE_PATH_STYLE
```

AWS SDK의 기본 credential provider chain도 지원한다.

`--source`는 다음 형식을 지원한다.

- `s3://bucket/key`
- `--bucket`과 `--key` 조합
- 로컬 archive 파일 경로

**`s3://` 형식과 `--bucket`/`--key`를 동시에 지정하면 에러로 거부한다 (둘 중 하나만 허용).**

로컬 archive 입력을 지원해 S3 없이도 테스트할 수 있게 한다.

## CLI 옵션

```
--source
    S3 URI 또는 로컬 archive 경로

--bucket, --key
    --source 대신 사용하는 S3 위치 지정 방식 (--source와 동시 사용 불가)

--target
    복원 대상 디렉터리

--dry-run
    기본 모드. 다운로드·압축 해제 계획만 출력하고 파일을 변경하지 않는다.

--confirm
    실제 파일 변경을 허용한다.

--mode=merge|replace
    기본값은 merge.

    merge:
      - 기존 target에 없는 파일은 새로 생성한다.
      - target과 archive에 동일 경로 파일이 모두 존재하면 archive 내용으로 덮어쓴다.
      - target에만 있고 archive에는 없는 파일은 삭제하지 않는다.

    replace:
      - target 전체를 archive 내용으로 교체한다.
      - 반드시 --confirm과 함께 사용해야 한다.

--staging-dir
    임시 압축 해제 디렉터리.
    기본값은 target과 동일한 파일시스템 상의 디렉터리
    (예: target의 부모 디렉터리 아래 `.coolrestore-staging-<timestamp>`)로 한다.
    staging-dir가 target과 다른 파일시스템에 있으면 최종 반영이 rename 기반 원자적 이동이 아니라
    copy+delete로 수행되므로, 이 경우 --verbose 여부와 무관하게 경고를 출력한다.

--skip-checksum
    다운로드한 archive에 대한 무결성 검증(S3 ETag 비교 또는 로컬 해시 비교)을 생략한다.
    기본값은 검증함(활성화).

--verbose
    상세 로그 출력
```

## 동작

1. source와 target을 검증한다.
2. target이 비어 있거나 위험한 경로인지 확인한다.
3. **target 기준으로 lock 파일을 획득한다. 이미 lock이 존재하면 즉시 에러로 종료한다 (동시 실행 방지).**
4. dry-run이 아니면 S3 archive를 임시 파일로 다운로드한다.
5. 로컬 source라면 해당 파일을 사용한다.
6. **다운로드/로컬 archive에 대해 무결성 검증을 수행한다 (S3 ETag 비교 또는 지정된 체크섬과 비교). `--skip-checksum`이 없으면 실패 시 즉시 중단하고 target을 변경하지 않는다.**
7. **staging-dir가 위치할 파티션의 여유 공간을 확인한다. archive 압축 해제 후 예상 크기보다 여유 공간이 부족하면 경고하고 중단한다 (강제 진행 옵션은 두지 않는다).**
8. gzip과 tar archive 형식을 검증한다.
9. archive 내부 경로를 검사한다.
10. 다음 경로를 거부한다.
    - 절대 경로
    - `../`를 통한 경로 탈출
    - target 밖으로 나가는 심볼릭 링크
    - 하드 링크
    - archive 내부의 위험한 특수 파일
11. staging directory에 먼저 압축을 해제한다.
12. 압축 해제가 성공한 뒤에만 target에 반영한다.
    - **merge 모드**: staging의 각 파일을 target의 동일 경로로 이동/복사한다. 이미 존재하는 파일은 덮어쓴다. target에만 있는 파일은 그대로 둔다.
    - **replace 모드**: `target`을 `target.bak-<timestamp>`로 rename한 뒤, staging 전체를 새 `target`으로 이동한다. 반영이 완전히 성공한 뒤에만 `target.bak-<timestamp>`를 삭제한다. 중간에 실패하면 `target.bak-<timestamp>`를 그대로 두고 원래 이름으로 rename을 시도해 최대한 복구한다.
13. merge 모드에서는 기존 파일을 삭제하지 않는다.
14. replace 모드에서는 기존 target을 안전하게 보존한 뒤 교체한다 (12번 절차 참조).
15. 복원 도중 실패하면 기존 target을 최대한 보존한다.
16. 임시 파일과 staging directory는 항상 정리한다 (성공/실패 무관, `.bak`은 replace 성공 시에만 정리).
17. **lock 파일을 해제한다 (정상 종료·에러 종료 모두 defer로 보장).**
18. 성공 시 source, target, mode, 복원 파일 수를 출력한다.
19. 실패 시 non-zero exit code를 반환한다.

## 안전 요구사항

- 기본 실행은 항상 dry-run이어야 한다.
- --confirm 없이는 파일을 변경하지 않는다.
- replace 모드는 별도 명시가 필요하다.
- target 경로가 `/`, `/etc`, `/var`, `/home` 등 위험한 경로이면 거부한다.
- archive path traversal을 차단한다.
- symlink와 hardlink는 기본적으로 거부한다.
- archive에 포함된 파일 권한을 무조건 신뢰하지 않는다.
- 자동으로 chown하지 않는다.
- UID/GID를 하드코딩하지 않는다 (복원된 파일은 실행 프로세스의 UID/GID를 따르며, 이 점을 README에 명시한다).
- 기존 파일 삭제 여부를 로그에 명확히 표시한다.
- S3 다운로드가 실패하면 target을 변경하지 않는다.
- **동일 target에 대한 동시 실행을 lock 파일로 차단한다.**
- **다운로드한 archive의 무결성을 검증한 뒤에만 압축 해제를 진행한다 (기본 활성화).**
- **staging 반영 전 대상 파티션 여유 공간을 확인한다.**

## 패키지 구조

```
cmd/coolrestore/main.go
internal/archive/
internal/restore/
internal/source/
internal/config/
internal/cli/
internal/lock/
tests 또는 각 패키지의 *_test.go
```

## 구현 분리

- **source**:
  - S3 source
  - local file source
  - 무결성 검증(체크섬/ETag 비교)
- **archive**:
  - gzip 해제
  - tar 검증
  - path traversal 검사
  - staging extraction
  - 여유 공간 확인
- **restore**:
  - merge mode (덮어쓰기 포함, 삭제 없음)
  - replace mode (rename 기반 교체, rollback-safe)
  - staging → target 반영이 rename 기반인지 여부 판단 (파일시스템 동일 여부)
- **lock**:
  - target 기준 lock 파일 획득/해제
- **cli**:
  - flag parsing
  - validation
  - output
  - exit code

## 테스트

실제 AWS S3에는 연결하지 않는다.

반드시 테스트할 항목:

1. 정상적인 s3:// URI 파싱
2. 로컬 archive source 처리
3. 잘못된 source 거부
4. 잘못된 target 거부
5. dry-run에서 파일 변경이 없는지 확인
6. confirm 없이 파일 변경이 없는지 확인
7. 정상적인 tar.gz archive 복원
8. merge 모드에서 기존 파일 보존 + 동일 경로 파일 덮어쓰기 확인
9. replace 모드에서 기존 파일 교체 및 `.bak` 정리 확인
10. `../` path traversal 차단
11. 절대 경로 차단
12. 위험한 symlink 차단
13. 손상된 gzip archive 거부
14. 손상된 tar archive 거부
15. S3 다운로드 실패 시 target이 변경되지 않는지 확인
16. 복원 실패 시 staging directory 정리
17. 성공 시 정확한 파일 수와 결과 출력
18. 실패 시 non-zero exit code 반환
19. **동일 target에 대해 두 프로세스가 동시 실행 시 하나는 lock 에러로 즉시 종료되는지 확인**
20. **손상되거나 예상 체크섬과 다른 archive를 무결성 검증 단계에서 거부하는지 확인**
21. **staging-dir와 target이 다른 파일시스템일 때 경고가 출력되는지 확인 (실제 다른 파일시스템 마운트는 필요 없고, 판단 로직 단위 테스트로 검증)**
22. **여유 공간 부족 시 압축 해제 전에 중단되는지 확인 (공간 확인 로직을 인터페이스로 추상화해 fake로 테스트)**
23. **replace 모드 도중 실패 시 `target.bak-<timestamp>`가 남아있고 원래 target 이름으로 최대한 복구되는지 확인**

## S3 adapter 테스트

- AWS SDK 호출을 추상화한다.
- S3 client를 직접 전역에서 생성하지 않는다.
- 테스트에서는 fake object source 또는 httptest server를 사용한다.
- 실제 AWS credential이 없어도 모든 테스트가 실행되어야 한다.

## 문서

README.md에 다음을 작성한다.

- 도구의 목적
- 지원하는 archive 형식
- Go 설치 없이 실행하는 방법
- S3 환경변수
- linux/arm64 바이너리 사용법
- dry-run 사용법
- merge와 replace 차이 (동일 경로 파일 덮어쓰기 여부 포함)
- 운영 복원 전에 web·worker·scheduler를 중지해야 한다는 점
- 복원 대상 경로를 확인해야 한다는 점
- DB 복원은 이 도구의 책임이 아니며 Coolify의 DB Import Backup을 사용한다는 점
- 비즈니스 데이터 정합성 검증은 별도 운영 절차라는 점
- 파일 권한과 소유권은 실행 환경(프로세스 UID/GID)에서 확인해야 한다는 점
- **staging-dir를 target과 다른 파일시스템에 두면 원자적 반영이 보장되지 않는다는 점**
- **동시 실행이 lock으로 차단된다는 점과, lock 파일 위치/수동 해제 방법**
- **무결성 검증 기본 동작과 `--skip-checksum`의 위험성**
- **Go 버전 요구사항 및 setup-go 미지원 시 폴백 설치 방법**

## 빌드

다음 대상을 지원한다.

```
GOOS=linux GOARCH=arm64
GOOS=linux GOARCH=amd64
```

GitHub Actions에서 다음을 실행한다.

- gofmt 검사
- go vet
- go test ./...
- go build
- 정적 바이너리 빌드
- SHA256 checksum 생성
- GitHub Release artifact 업로드

## 최종 결과

- 실제 구현 파일
- go.mod / go.sum
- 테스트
- README
- GitHub Actions build workflow
- arm64와 amd64 바이너리
- 실행 예시
- 테스트 결과
- 알려진 제한사항

구현 전에 먼저 설계와 파일 구조를 요약하고, 구현 후 테스트를 실행하라.
