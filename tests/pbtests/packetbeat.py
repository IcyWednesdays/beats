import subprocess
import jinja2
import unittest
import os
import shutil
import json
import time
from datetime import datetime, timedelta


class Proc(object):
    """
    Slim wrapper on subprocess.Popen that redirects
    both stdout and stderr to a file on disk and makes
    sure to stop the process and close the output file when
    the object gets collected.
    """
    def __init__(self, args, outputfile):
        self.args = args
        self.output = open(outputfile, "wb")

    def start(self):
        self.proc = subprocess.Popen(
            self.args,
            stdout=self.output,
            stderr=subprocess.STDOUT)
        return self.proc

    def wait(self):
        self.proc.wait()

    def kill_and_wait(self):
        self.proc.terminate()
        self.proc.wait()

    def __del__(self):
        try:
            self.output.close()
        except:
            pass
        try:
            self.proc.terminate()
            self.proc.kill()
        except:
            pass


class TestCase(unittest.TestCase):

    def run_packetbeat(self, pcap,
                       cmd="../packetbeat",
                       config="packetbeat.conf",
                       output="packetbeat.log",
                       extra_args=[],
                       debug_selectors=[]):
        """
        Executes packetbeat on an input pcap file.
        Waits for the process to finish before returning to
        the caller.
        """

        args = [cmd]

        args.extend(["-e",
                     "-I", os.path.join("pcaps", pcap),
                     "-c", os.path.join(self.working_dir, config),
                     "-t"])
        if extra_args:
            args.extend(extra_args)

        if debug_selectors:
            args.extend(["-d", ",".join(debug_selectors)])

        with open(os.path.join(self.working_dir, output), "wb") as outputfile:
            proc = subprocess.Popen(args,
                                    stdout=outputfile,
                                    stderr=subprocess.STDOUT)
            proc.wait()

    def start_packetbeat(self,
                         cmd="../packetbeat",
                         config="packetbeat.conf",
                         output="packetbeat.log",
                         extra_args=[],
                         debug_selectors=[]):
        """
        Starts packetbeat and returns the process handle. The
        caller is responsible for stopping / waiting for the
        Proc instance.
        """
        args = [cmd,
                "-e",
                "-c", os.path.join(self.working_dir, config)]
        if extra_args:
            args.extend(extra_args)

        if debug_selectors:
            args.extend(["-d", ",".join(debug_selectors)])

        proc = Proc(args, os.path.join(self.working_dir, output))
        proc.start()
        return proc

    def render_config_template(self, template="packetbeat.conf.j2",
                               output="packetbeat.conf", **kargs):
        template = self.template_env.get_template(template)
        kargs["pb"] = self
        output_str = template.render(**kargs)
        with open(os.path.join(self.working_dir, output), "wb") as f:
            f.write(output_str)

    def read_output(self, output_file="output/packetbeat"):
        jsons = []
        with open(os.path.join(self.working_dir, output_file), "r") as f:
            for line in f:
                jsons.append(json.loads(line))
        return jsons

    def copy_files(self, files, source_dir="files/"):
        for file_ in files:
            shutil.copy(os.path.join(source_dir, file_),
                        self.working_dir)

    def setUp(self):

        self.template_env = jinja2.Environment(
            loader=jinja2.FileSystemLoader("templates")
        )

        # create working dir
        self.working_dir = os.path.join("run", self.id())
        if os.path.exists(self.working_dir):
            shutil.rmtree(self.working_dir)
        os.makedirs(self.working_dir)

        # update the last_run link
        if os.path.islink("last_run"):
            os.unlink("last_run")
        os.symlink("run/{}".format(self.id()), "last_run")

    def wait_until(self, cond, max_timeout=10, poll_interval=0.1):
        """
        Waits until the cond function returns true,
        or until the max_timeout is reached. Calls the cond
        function every poll_interval seconds.

        If the max_timeout is reached before cond() returns
        true, an exception is raised.
        """
        start = datetime.now()
        while not cond():
            if datetime.now() - start > timedelta(seconds=max_timeout):
                raise Exception("Timeout waiting for condition. " +
                                "Waited {} seconds".format(max_timeout))
            time.sleep(poll_interval)

    def log_contains(self, msg, logfile="packetbeat.log"):
        """
        Returns true if the give logfile contains the given message.
        Note that the msg must be present in a single line.
        """
        with open(os.path.join(self.working_dir, logfile), "r") as f:
            for line in f:
                if line.find(msg) >= 0:
                    return True
            return False
