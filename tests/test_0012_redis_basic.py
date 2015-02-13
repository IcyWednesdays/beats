from pbtests.packetbeat import TestCase


class Test(TestCase):
    """
    Basic REDIS tests
    """

    def test_redis_session(self):
        """
        Should correctly pass a simple Redis Session containing
        also an error.
        """
        self.render_config_template(
            redis_ports=[6380]
        )
        self.run_packetbeat(pcap="redis_session.pcap")

        objs = self.read_output()
        assert all([o["type"] == "redis" for o in objs])

        assert objs[0]["method"] == "SET"
        assert objs[0]["path"] == "key3"
        assert objs[0]["query"] == "set key3 me"
        assert objs[0]["status"] == "OK"
        assert objs[0]["redis"]["response"] == "OK"

        assert objs[1]["status"] == "OK"
        assert objs[1]["method"] == "GET"
        assert objs[1]["redis"]["response"] == "me"
        assert objs[1]["query"] == "get key3"
        assert objs[1]["redis"]["response"] == "me"

        assert objs[2]["status"] == "Error"
        assert objs[2]["method"] == "LLEN"
        assert objs[2]["redis"]["error"] == "ERR Operation against a key " + \
            "holding the wrong kind of value"

        # the rest should be successful
        assert all([o["status"] == "OK" for o in objs[3:]])
        assert all(["response" in o["redis"] for o in objs[3:]])
        assert all([isinstance(o["method"], basestring) for o in objs[3:]])
        assert all([isinstance(o["path"], basestring) for o in objs[3:]])
        assert all([isinstance(o["query"], basestring) for o in objs[3:]])
