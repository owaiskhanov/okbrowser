# Authenticode signing

OK Browser's release workflow supports trusted Authenticode signing without
committing a certificate or password to the repository.

## Required repository secrets

Configure these under **GitHub repository → Settings → Secrets and variables →
Actions**:

| Secret | Value |
|---|---|
| `WINDOWS_CERTIFICATE_BASE64` | Base64 representation of the complete `.pfx`/PKCS#12 certificate file |
| `WINDOWS_CERTIFICATE_PASSWORD` | Password protecting that PFX file |

Do not paste either value into issues, pull requests, source files, build logs,
or chat.

To encode a PFX locally with PowerShell:

```powershell
[Convert]::ToBase64String([IO.File]::ReadAllBytes("publisher.pfx")) |
  Set-Content -NoNewline certificate-base64.txt
```

Copy the content of `certificate-base64.txt` into the GitHub secret, then delete
the temporary text file securely.

## Release behavior

When both secrets are configured, each Windows CI leg:

1. Builds `OKBrowser.exe` from source.
2. Decodes the certificate only into the hosted runner's temporary directory.
3. Signs with SHA-256 using Microsoft's `signtool`.
4. Adds a trusted RFC 3161 timestamp through DigiCert.
5. Runs `signtool verify /pa /all /v` and fails closed if verification fails.
6. Runs the normal browser smoke tests against the signed executable.
7. Deletes the temporary PFX even if signing fails.
8. Publishes the signed, tested executable with its SHA-256 checksum and GitHub
   build-provenance attestation.

If either secret is absent, CI explicitly reports that the build is unsigned and
continues publishing the source-verified build. It never generates or presents
a self-signed certificate as a trusted publisher signature.

## Certificate choice

Use an OV or EV code-signing certificate issued by a CA trusted by Windows.
Authenticode confirms publisher identity and protects file integrity. SmartScreen
reputation is controlled by Microsoft and can still take time to build,
particularly with a new OV certificate. EV certificates generally establish
reputation faster but do not replace safe-distribution practices.
