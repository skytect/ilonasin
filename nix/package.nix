{
  lib,
  buildGoModule,
  pkg-config,
  sqlite,
}:

buildGoModule {
  pname = "ilonasin";
  version = "0.1.0";

  src = lib.cleanSource ../.;

  vendorHash = "sha256-gBP25CkwLRKRuZQB/2E/5JwE29GBrmJe8pe49hXBVpw=";

  subPackages = [ "cmd/ilonasin" ];

  nativeBuildInputs = [ pkg-config ];
  buildInputs = [ sqlite ];

  env.CGO_ENABLED = 1;

  ldflags = [
    "-s"
    "-w"
  ];

  meta = {
    description = "Local OpenAI-compatible LLM router";
    homepage = "https://github.com/skytect/ilonasin";
    license = lib.licenses.mit;
    mainProgram = "ilonasin";
  };
}
