# frozen_string_literal: true

require_relative "package_test_helper"
require "rubygems/package"

class PackagingTest < Minitest::Test
  include PackageTestHelpers

  def test_rejects_versions_that_could_escape_paths_or_change_shell_commands
    assert_equal "1.18.3", Packages.version_arg("v1.18.3")
    ["../1.2.3", "1.2", "1.2.3;id", "1.2.3\n", "1.2.3-rc1", "01.2.3"].each do |version|
      assert_raises(Packages::Error, version) { Packages.version_arg(version) }
    end
  end

  def test_missing_or_corrupted_assets_fail_verification
    in_temporary_directory do |root|
      asset = root / "release.tar.gz"
      asset.write("original release")
      checksums = root / "checksums.txt"
      digest = OpenSSL::Digest::SHA256.file(asset).hexdigest
      checksums.write("#{digest}  #{asset.basename}\n")
      Packages.verify_assets(checksums, [asset])
      asset.write("corrupted download")
      error = assert_raises(Packages::Error) { Packages.verify_assets(checksums, [asset]) }
      assert_match "checksum mismatch", error.message
      checksums.write("")
      error = assert_raises(Packages::Error) { Packages.verify_assets(checksums, [asset]) }
      assert_match "missing entry", error.message
      checksums.write("#{digest}\n")
      assert_raises(Packages::Error) { Packages.verify_assets(checksums, [asset]) }
      checksums.write("#{digest}  #{asset.basename}\n" * 2)
      assert_raises(Packages::Error) { Packages.verify_assets(checksums, [asset]) }
    end
  end

  def test_hash_algorithms_match_known_vectors
    in_temporary_directory do |root|
      file = root / "abc"
      file.write("abc")
      hashes = Packages.digests(file)
      assert_equal "900150983cd24fb0d6963f7d28e17f72", hashes.fetch("md5")
      assert_equal "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", hashes.fetch("sha256")
      assert_equal "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192" \
                   "992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f", hashes.fetch("sha512")
      assert_equal "ba80a53f981c4d0d6a2797b69f12f6e94c212f14685ac4b74b12bb6fdbffa2d17d87" \
                   "c5392aab792dc252d5de4533cc9518d38aa8dbf1925ab92386edd4009923", hashes.fetch("blake2b")
    end
  end

  def test_unknown_template_tokens_fail
    assert_raises(KeyError) { Packages.render("version=@VERISON@", "VERSION" => "1.2.3") }
  end

  def test_new_version_updates_every_recipe_and_detects_drift
    in_temporary_directory do |output|
      Packages.generate(output, metadata)
      Packages.check(output) # Also compares .SRCINFO with real makepkg when installed.
      %w[alpine/APKBUILD debian/changelog rpm/hyprmoncfg.spec slackware/hyprmoncfg.info void/template nix/default.nix].each do |path|
        assert_includes (output / path).read, "9.8.7"
      end
      ebuild = output / "gentoo/gui-apps/hyprmoncfg/hyprmoncfg-9.8.7.ebuild"
      assert_path_exists ebuild
      assert_includes (ebuild.dirname / "Manifest").read, "hyprmoncfg-9.8.7-deps.tar.xz"
      binary = output / "gentoo/gui-apps/hyprmoncfg-bin/hyprmoncfg-bin-9.8.7.ebuild"
      assert_includes binary.read, "doicon packaging/icons/hyprmoncfg.svg"
      stable = (output / "arch/hyprmoncfg/PKGBUILD").read
      assert_includes stable, 'GOMODCACHE="${srcdir}/go-mod" GOPROXY=off'
      vcs = (output / "arch/hyprmoncfg-git/PKGBUILD").read
      assert_includes vcs, '$(git rev-parse --short=7 HEAD)'
      refute_includes vcs, "GOPROXY=off"
      first = snapshot(output)
      Packages.generate(output, metadata)
      assert_equal first, snapshot(output)
      (output / "rpm/hyprmoncfg.spec").write("Version: 0.0.1\n")
      error = assert_raises(Packages::Error) { Packages.check(output) }
      assert_match "stale", error.message
    end
  end

  def test_archive_extraction_rejects_parent_traversal
    in_temporary_directory do |root|
      archive = root / "malicious.tar"
      File.open(archive, "wb") do |file|
        Gem::Package::TarWriter.new(file) do |tar|
          tar.add_file_simple("../outside", 0o644, 3) { |entry| entry.write("bad") }
        end
      end
      destination = root / "extracted"
      destination.mkpath
      capture_subprocess_io do
        assert_raises(Packages::Error) { Packages.extract_archive(archive, destination) }
      end
      refute_path_exists root / "outside"
    end
  end

  def test_source_recipe_assets_have_separate_verified_checksums
    in_temporary_directory do |root|
      recipes, output = root / "recipes", root / "source assets"
      Packages.generate(recipes, metadata)
      Packages.artifacts(recipes, output: output)
      archive = output / "hyprmoncfg-9.8.7-packaging.tar.xz"
      assert_path_exists archive
      assert_equal "#{Packages.sha256(archive)}  #{archive.basename}\n",
        (output / "source-packaging-checksums.txt").read
      refute_path_exists output / "packaging-checksums.txt"
      assert_raises(Packages::Error) { Packages.artifacts(recipes, output: output) }
    end
  end

  def test_cli_reports_invalid_arguments_and_preserves_existing_output
    script = Packages::ROOT / "scripts/package_sources.rb"
    stdout, stderr, status = Open3.capture3(RbConfig.ruby, script.to_s, "--help")
    assert status.success?
    assert_includes stdout, "prepare VERSION"
    assert_empty stderr

    _, stderr, status = Open3.capture3(RbConfig.ruby, script.to_s, "prepare", "1.2.3;id")
    refute status.success?
    assert_includes stderr, "expected a stable version"

    in_temporary_directory do |root|
      output = root / "output with spaces"
      output.mkpath
      (output / "keep.txt").write("keep me")
      _, stderr, status = Open3.capture3(RbConfig.ruby, script.to_s, "prepare", "1.2.3", "--output", output.to_s)
      refute status.success?
      assert_includes stderr, "output already exists"
      assert_equal "keep me", (output / "keep.txt").read
    end
  end
end
